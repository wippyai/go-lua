package summary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func TestHeapTableObjectsCompareAndDigestStructurallyAcrossKeySpaces(t *testing.T) {
	reg := mustRegistry(t)
	id := identity.ID{Kind: "table", Site: "keyspace-determinism", Index: 1}
	leftKS, rightKS := keyspace.New(), keyspace.New()
	if _, ok := rightKS.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "padding"}}); !ok {
		t.Fatal("right padding suffix failed")
	}
	leftKey := mustRootlessSuffix(t, leftKS, "name")
	rightKey := mustRootlessSuffix(t, rightKS, "name")
	if leftKey.Segs == rightKey.Segs {
		t.Fatal("test setup did not shift dense segment ids")
	}
	makeSummary := func(ks *keyspace.KeySpace, key keyspace.Key) Summary {
		return Summary{
			HeapKeySpace: ks,
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
					Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{key: product.Top()},
				}),
			},
		}
	}
	left, right := makeSummary(leftKS, leftKey), makeSummary(rightKS, rightKey)
	if !EqualNormalized(reg, left, right) || !LessOrEq(reg, left, right) || !LessOrEq(reg, right, left) {
		t.Fatal("structurally equal heap objects differed across keyspaces")
	}
	if leftDigest, rightDigest := NormalizedPayloadDigest(reg, left), NormalizedPayloadDigest(reg, right); leftDigest != rightDigest {
		t.Fatalf("structurally equal heap digests differ: left=%d right=%d", leftDigest, rightDigest)
	}
}

func TestRekeyHeapTableObjectsImportsDynamicTableKeys(t *testing.T) {
	id := identity.ID{Kind: "table", Site: "dynamic-rekey", Index: 1}
	from, to := keyspace.New(), keyspace.New()
	if _, ok := to.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "padding"}}); !ok {
		t.Fatal("target padding suffix failed")
	}
	fromTable := mustRootlessSuffix(t, from, "items")
	fact := dynamicindex.Fact{
		KeyPresence: presence.Present(), KeyValue: product.Top(), Value: product.Top(), Admission: dynamicindex.AdmissionAdmitted,
	}
	s := Summary{
		HeapKeySpace: from,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: product.Top(),
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
					{Table: fromTable, Site: "write"}: fact,
				},
			}),
		},
	}
	rekeyed, err := s.RekeyHeapTableObjects(to)
	if err != nil {
		t.Fatal(err)
	}
	object := rekeyed.HeapTableObjects[id]
	facts := object.DynamicIndexFacts()
	if len(facts) != 1 {
		t.Fatalf("dynamic facts after rekey = %#v", facts)
	}
	for key := range facts {
		if got := to.Format(key.Table); got != ".items" {
			t.Fatalf("dynamic table key after rekey = %q, want .items", got)
		}
	}
}

func TestRekeyHeapTableObjectsRejectsForeignNestedKeyTransactionally(t *testing.T) {
	id := identity.ID{Kind: "table", Site: "foreign-dynamic-rekey", Index: 1}
	owner, foreign, target := keyspace.New(), keyspace.New(), keyspace.New()
	foreignTable := mustRootlessSuffix(t, foreign, "items")
	fact := dynamicindex.Fact{
		KeyPresence: presence.Present(), KeyValue: product.Top(), Value: product.Top(), Admission: dynamicindex.AdmissionAdmitted,
	}
	original := Summary{
		HeapKeySpace: owner,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: product.Top(),
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
					{Table: foreignTable, Site: "write"}: fact,
				},
			}),
		},
	}

	got, err := original.RekeyHeapTableObjects(target)
	if err == nil {
		t.Fatal("foreign nested key was accepted")
	}
	if got.HeapKeySpace != owner {
		t.Fatal("failed import relabeled heap keyspace")
	}
	if facts := got.HeapTableObjects[id].DynamicIndexFacts(); len(facts) != 1 {
		t.Fatalf("failed import partially erased dynamic facts: %#v", facts)
	}
}

func TestRekeyHeapTableObjectsReturnsErrorForMissingProvenance(t *testing.T) {
	id := identity.ID{Kind: "table", Site: "missing-provenance", Index: 1}
	member := mustRootlessSuffix(t, keyspace.New(), "member")
	s := Summary{
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{member: product.Top()},
			}),
		},
	}
	if got, err := s.RekeyHeapTableObjects(keyspace.New()); err == nil || got.HeapKeySpace != nil {
		t.Fatalf("missing provenance rekey = keyspace:%p err:%v, want original and error", got.HeapKeySpace, err)
	}
}

func TestRekeyHeapTableObjectsAllowsNilOnlyForKeyFreePayload(t *testing.T) {
	id := identity.ID{Kind: "table", Site: "key-free", Index: 1}
	s := Summary{HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
	}}
	got, err := s.RekeyHeapTableObjects(nil)
	if err != nil {
		t.Fatalf("key-free nil rekey failed: %v", err)
	}
	if got.HeapKeySpace != nil || len(got.HeapTableObjects) != 1 {
		t.Fatalf("key-free nil rekey = keyspace:%p objects:%d", got.HeapKeySpace, len(got.HeapTableObjects))
	}

	valid := keyspace.New()
	copyValue := *valid
	invalid := &copyValue
	if _, err := s.RekeyHeapTableObjects(invalid); err == nil {
		t.Fatal("key-free summary accepted invalid nonnil target authority")
	}
}

func TestRekeyHeapTableObjectsRejectsInvalidAuthorityForEmptyPayload(t *testing.T) {
	valid := keyspace.New()
	copyValue := *valid
	invalid := &copyValue
	if _, err := (Summary{}).RekeyHeapTableObjects(invalid); err == nil {
		t.Fatal("empty summary accepted invalid target authority")
	}
	if _, err := (Summary{HeapKeySpace: invalid}).RekeyHeapTableObjects(nil); err == nil {
		t.Fatal("empty summary accepted stale invalid source authority")
	}
}

func TestHeapTableObjectLatticeRejectsSharedInvalidKeySpace(t *testing.T) {
	reg := mustRegistry(t)
	valid := keyspace.New()
	copyValue := *valid
	invalid := &copyValue
	id := identity.ID{Kind: "table", Site: "invalid-shared-keyspace", Index: 1}
	s := Summary{
		HeapKeySpace: invalid,
		HeapTableObjects: map[identity.ID]heapidentity.TableObject{
			id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
		},
	}
	if summaryHeapTableObjectsEqual(reg, s, s) {
		t.Fatal("equality accepted shared invalid keyspace")
	}
	if summaryHeapTableObjectsLessOrEq(reg, s, s) {
		t.Fatal("less-or-equal accepted shared invalid keyspace")
	}
	for _, test := range []struct {
		name string
		run  func()
	}{
		{name: "join", run: func() { _, _ = joinSummaryHeapTableObjects(reg, s, s) }},
		{name: "widen", run: func() { _, _ = widenSummaryHeapTableObjects(reg, s, s) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s accepted shared invalid keyspace", test.name)
				}
			}()
			test.run()
		})
	}
}

func TestSummaryDigestRejectsDenseHeapKeyWithoutProducingKeySpace(t *testing.T) {
	reg := mustRegistry(t)
	ks := keyspace.New()
	key := mustRootlessSuffix(t, ks, "name")
	s := Summary{HeapTableObjects: map[identity.ID]heapidentity.TableObject{
		{Kind: "table", Site: "missing-keyspace", Index: 1}: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{key: product.Top()},
		}),
	}}
	defer func() {
		if recover() == nil {
			t.Fatal("digest accepted solve-local dense heap key without producing keyspace")
		}
	}()
	_ = NormalizedPayloadDigest(reg, s)
}
