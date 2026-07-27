package state

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
)

func TestDefaultLaneCatalogHasSemanticFingerprintCoverage(t *testing.T) {
	for _, spec := range defaultLaneCatalog.specs {
		if spec.fingerprint == nil {
			t.Errorf("registered lane %q has no semantic fingerprint", spec.id)
		}
	}
}

func TestLaneCatalogRejectsMissingSemanticFingerprintAtConstruction(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("catalog accepted lane without semantic fingerprint")
		}
	}()
	_ = newLaneCatalog([]laneSpec{{id: "missing-fingerprint"}})
}

func TestSemanticFingerprintSeparatesEveryRegisteredLane(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	for _, sample := range stateLawLaneSamples(reg, keys) {
		t.Run(string(sample.lane), func(t *testing.T) {
			lanes := []LaneID{sample.lane}
			domain := DomainWithLanes(reg, lanes)
			bottom := domain.Bottom()
			if domain.Equal(bottom, sample.state) {
				t.Fatal("lane sample equals selected-domain bottom")
			}
			bottomDigest := fingerprintForTest(t, reg, keys, lanes, bottom)
			sampleDigest := fingerprintForTest(t, reg, keys, lanes, sample.state)
			if bottomDigest == sampleDigest {
				t.Fatalf("selected lane shares bottom fingerprint %d", sampleDigest)
			}
			if got := fingerprintForTest(t, reg, keys, lanes, sample.state); got != sampleDigest {
				t.Fatalf("repeated fingerprint = %d, want deterministic %d", got, sampleDigest)
			}
			emptyLanes := []LaneID{}
			if got, want := fingerprintForTest(t, reg, keys, emptyLanes, sample.state), fingerprintForTest(t, reg, keys, emptyLanes, bottom); got != want {
				t.Fatalf("disabled lane changed fingerprint: %d != %d", got, want)
			}
		})
	}
}

func TestSemanticFingerprintHeapObjectCoversEverySemanticSubfield(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	id := identity.ID{Kind: "lua.table", Site: "fingerprint-subfields", Index: 1}
	root := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	otherRoot := product.Absent(reg)
	staticKey, ok := heapidentity.StaticMemberSuffixKey(keys, keys.Segments(mustStateKey(t, keys, pathdom.PathKey("sym1@1.member"))))
	if !ok {
		t.Fatal("static suffix key")
	}
	tableKey := mustStateKey(t, keys, pathdom.PathKey("sym1@1.table"))
	dynamicKey := dynamicindex.Key{Table: tableKey, Site: "write"}
	dynamicFact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		KeyValue: root, HasKeyValue: true,
		Value: root, HasValue: true,
		Admission: dynamicindex.AdmissionAdmitted,
	})
	objects := map[string]heapidentity.TableObject{
		"root": heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: otherRoot}),
		"static-member": heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: root, StaticMembers: map[keyspace.Key]product.Value{staticKey: root},
		}),
		"dynamic-index": heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: root, DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{dynamicKey: dynamicFact},
		}),
		"stable-shape":        heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, StableShape: true}),
		"prefix-stable-shape": heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, PrefixStableShape: true}),
	}
	base := State{}.WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root}))
	baseDigest := fingerprintForTest(t, reg, keys, []LaneID{LaneHeapTableIdentity}, base)
	seen := map[uint64]string{baseDigest: "base"}
	domain := DomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	for name, object := range objects {
		variant := State{}.WriteHeapTableObject(reg, id, object)
		if domain.Equal(base, variant) {
			t.Fatalf("%s variant equals base", name)
		}
		digest := fingerprintForTest(t, reg, keys, []LaneID{LaneHeapTableIdentity}, variant)
		if prior, ok := seen[digest]; ok {
			t.Fatalf("%s and %s share fingerprint %d", name, prior, digest)
		}
		seen[digest] = name
	}
}

func TestSemanticFingerprintCanonicalizesEquivalentStateSpellings(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := Domain(reg)
	left := State{}
	right := NormalizeForDomain(domain, left)
	if !domain.Equal(left, right) {
		t.Fatal("test spellings are not semantically equal")
	}
	if got, want := fingerprintForTest(t, reg, keys, nil, left), fingerprintForTest(t, reg, keys, nil, right); got != want {
		t.Fatalf("equivalent spellings fingerprint differently: %d != %d", got, want)
	}
}

func TestSemanticFingerprintAgreesWithDomainEquality(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	samples := stateLawSample(reg, keys)
	for _, lanes := range [][]LaneID{nil, {}, {LanePathEvidence}, {LaneHeapTableIdentity}, {LaneKeyMemberships}} {
		domain := DomainWithOptionalLanes(reg, lanes)
		for i, left := range samples {
			for j, right := range samples {
				if !domain.Equal(left, right) {
					continue
				}
				leftDigest := fingerprintForTest(t, reg, keys, lanes, left)
				rightDigest := fingerprintForTest(t, reg, keys, lanes, right)
				if leftDigest != rightDigest {
					t.Fatalf("lanes=%v equal samples %d/%d fingerprint differently: %d != %d", lanes, i, j, leftDigest, rightDigest)
				}
			}
		}
	}
}

func TestSemanticFingerprintCanonicalizesLaneSetAndMapOrder(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	firstID := identity.ID{Kind: "lua.table", Site: "order", Index: 1}
	secondID := identity.ID{Kind: "lua.table", Site: "order", Index: 2}
	root := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	firstObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, PrefixStableShape: true})
	secondObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)})
	left := State{}.
		WriteHeapTableObject(reg, firstID, firstObject).
		WriteHeapTableObject(reg, secondID, secondObject)
	right := State{}.
		WriteHeapTableObject(reg, secondID, secondObject).
		WriteHeapTableObject(reg, firstID, firstObject)
	if !Domain(reg).Equal(left, right) {
		t.Fatal("reverse insertion states differ")
	}
	if got, want := fingerprintForTest(t, reg, keys, nil, left), fingerprintForTest(t, reg, keys, nil, right); got != want {
		t.Fatalf("map insertion order changed fingerprint: %d != %d", got, want)
	}
	reversedLanes := DefaultLanes()
	for i, j := 0, len(reversedLanes)-1; i < j; i, j = i+1, j-1 {
		reversedLanes[i], reversedLanes[j] = reversedLanes[j], reversedLanes[i]
	}
	if got, want := fingerprintForTest(t, reg, keys, reversedLanes, left), fingerprintForTest(t, reg, keys, nil, left); got != want {
		t.Fatalf("lane input order changed fingerprint: %d != %d", got, want)
	}
}

func TestSemanticFingerprintCanonicalizesPathMembershipOrderWithoutContainer(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	firstKey := mustTestStateKey(pathdom.PathKey("sym901@1.key"))
	firstTable := mustTestStateKey(pathdom.PathKey("sym901@1.table"))
	secondKey := mustTestStateKey(pathdom.PathKey("sym902@1.key"))
	secondTable := mustTestStateKey(pathdom.PathKey("sym902@1.table"))
	left := State{}.
		AddPathKeyMembership(firstKey, firstTable).
		AddPathKeyMembership(secondKey, secondTable)
	right := State{}.
		AddPathKeyMembership(secondKey, secondTable).
		AddPathKeyMembership(firstKey, firstTable)

	if got, want := fingerprintForTest(t, reg, keys, []LaneID{LaneKeyMemberships}, left), fingerprintForTest(t, reg, keys, []LaneID{LaneKeyMemberships}, right); got != want {
		t.Fatalf("path-membership insertion order changed fingerprint: %d != %d", got, want)
	}
}

func TestSemanticFingerprintObservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SemanticFingerprint(FingerprintConfig{
		Context: ctx, Registry: standard.Registry(), KeySpace: keyspace.New(),
	}, State{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SemanticFingerprint error = %v, want cancellation", err)
	}
}

func TestSemanticFingerprintUsesLosslessStructuralPathKeys(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	fieldWithDot, ok := keys.FromResolverKey(1, 1, []segment.Segment{{Kind: segment.SegmentField, Name: "a.b"}})
	if !ok {
		t.Fatal("field-with-dot key")
	}
	twoFields, ok := keys.FromResolverKey(1, 1, []segment.Segment{
		{Kind: segment.SegmentField, Name: "a"},
		{Kind: segment.SegmentField, Name: "b"},
	})
	if !ok {
		t.Fatal("two-fields key")
	}
	if got, want := keys.FormatReadOnly(fieldWithDot), keys.FormatReadOnly(twoFields); got != want {
		t.Fatalf("test requires colliding display spellings: %q != %q", got, want)
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	build := func(firstKey keyspace.Key, firstValue product.Value, secondKey keyspace.Key, secondValue product.Value) State {
		lane, _ := (pathevidence.Lane{}).WritePathKey(reg, firstKey, firstValue)
		lane, _ = lane.WritePathKey(reg, secondKey, secondValue)
		return State{pathEvidence: lane}
	}
	left := build(fieldWithDot, present, twoFields, absent)
	right := build(twoFields, absent, fieldWithDot, present)
	if got, want := fingerprintForTest(t, reg, keys, []LaneID{LanePathEvidence}, left), fingerprintForTest(t, reg, keys, []LaneID{LanePathEvidence}, right); got != want {
		t.Fatalf("insertion order changed fingerprint: %d != %d", got, want)
	}
	swapped := build(fieldWithDot, absent, twoFields, present)
	if got, other := fingerprintForTest(t, reg, keys, []LaneID{LanePathEvidence}, left), fingerprintForTest(t, reg, keys, []LaneID{LanePathEvidence}, swapped); got == other {
		t.Fatalf("distinct structural key/value associations share fingerprint %d", got)
	}
}

func TestSemanticFingerprintBranchProofOptionalOtherIsValid(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	path := mustStateKey(t, keys, pathdom.PathKey("sym1@1.path"))
	other := mustStateKey(t, keys, pathdom.PathKey("sym1@1.other"))
	withoutOther := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: path}
	withOther := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: path, Other: other}
	left := State{}.AddBranchProof(withoutOther).AddBranchProof(withOther)
	right := State{}.AddBranchProof(withOther).AddBranchProof(withoutOther)
	if got, want := fingerprintForTest(t, reg, keys, []LaneID{LanePathEvidence}, left), fingerprintForTest(t, reg, keys, []LaneID{LanePathEvidence}, right); got != want {
		t.Fatalf("optional Other ordering changed fingerprint: %d != %d", got, want)
	}
}

func TestSemanticFingerprintFailsClosedOnOutOfRangeForeignKey(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	foreign := keyspace.New()
	foreignKey, ok := foreign.FromResolverKey(1, 1, []segment.Segment{{Kind: segment.SegmentField, Name: "foreign"}})
	if !ok {
		t.Fatal("foreign key")
	}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	lane, _ := (pathevidence.Lane{}).WritePathKey(reg, foreignKey, value)
	_, err := SemanticFingerprint(FingerprintConfig{Registry: reg, KeySpace: keys, Lanes: []LaneID{LanePathEvidence}}, State{pathEvidence: lane})
	if !errors.Is(err, ErrFingerprintKeySpace) {
		t.Fatalf("SemanticFingerprint error = %v, want ErrFingerprintKeySpace", err)
	}
}

func TestSemanticFingerprintFailsClosedOnUnknownUserLatticeAxis(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	path := mustStateKey(t, keys, pathdom.PathKey("sym1@1.path"))
	st := State{userLattices: userLatticeLane{values: map[userLatticeKey]userlattice.Element{
		{axis: 0, path: path}: 0,
	}}}
	_, err := SemanticFingerprint(FingerprintConfig{Registry: reg, KeySpace: keys, Lanes: []LaneID{LaneUserLattices}}, st)
	if !errors.Is(err, ErrFingerprintCoverage) {
		t.Fatalf("SemanticFingerprint error = %v, want ErrFingerprintCoverage", err)
	}
}

func TestSemanticFingerprintConcurrentReadOnlyKeySpace(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	lane := pathevidence.Lane{}
	for i := 1; i <= 64; i++ {
		key, ok := keys.FromResolverKey(1, 1, []segment.Segment{{Kind: segment.SegmentIndexInt, Index: i}})
		if !ok {
			t.Fatalf("key %d", i)
		}
		lane, _ = lane.WritePathKey(reg, key, value)
	}
	st := State{pathEvidence: lane}
	config := FingerprintConfig{Registry: reg, KeySpace: keys, Lanes: []LaneID{LanePathEvidence}}
	want, err := SemanticFingerprint(config, st)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				got, fingerprintErr := SemanticFingerprint(config, st)
				if fingerprintErr != nil {
					errs <- fingerprintErr
					return
				}
				if got != want {
					errs <- errors.New("concurrent fingerprint changed")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func BenchmarkSemanticFingerprintFullState(b *testing.B) {
	reg := standard.Registry()
	keys := keyspace.New()
	samples := stateLawSample(reg, keys)
	full := samples[len(samples)-1]
	config := FingerprintConfig{Registry: reg, KeySpace: keys}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := SemanticFingerprint(config, full); err != nil {
			b.Fatal(err)
		}
	}
}

func fingerprintForTest(t *testing.T, reg *axis.Registry, keys *keyspace.KeySpace, lanes []LaneID, st State) uint64 {
	t.Helper()
	digest, err := SemanticFingerprint(FingerprintConfig{Registry: reg, KeySpace: keys, Lanes: lanes}, st)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
