package pathevidence

import (
	"sort"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCoordinatePathClosureReverseOrderedChainHasLinearAllocationGrowth(t *testing.T) {
	measure := func(width int) float64 {
		coordinates, seed := reverseOrderedCoordinateEqualityChain(t, width)
		selected, ok, work := coordinatePathClosureWithWork(coordinates, seed.owner, []keyspace.Key{seed.key}, nil)
		if !ok || len(selected) != width {
			t.Fatalf("closure width %d = %d/%v", width, len(selected), ok)
		}
		for index, present := range selected {
			if !present {
				t.Fatalf("closure width %d omitted reverse-chain coordinate %d", width, index)
			}
		}
		wantWork := coordinatePathClosureWork{
			coordinateSelections: width,
			pathActivations:      width,
			adjacencyVisits:      width * 2,
			trieNodeExpansions:   width + 1,
		}
		if work != wantWork {
			t.Fatalf("closure width %d work = %+v, want %+v", width, work, wantWork)
		}
		return testing.AllocsPerRun(5, func() {
			selected, ok := CoordinatePathClosure(coordinates, seed.owner, []keyspace.Key{seed.key})
			if !ok || len(selected) != width || !selected[0] || !selected[width-1] {
				panic("reverse-chain closure changed during allocation measurement")
			}
		})
	}

	small := measure(48)
	large := measure(96)
	t.Logf("reverse-chain allocations: width 48=%g width 96=%g", small, large)
	if large > small*3 {
		t.Fatalf("reverse-chain allocations grew superlinearly: width 48=%g width 96=%g", small, large)
	}
}

type coordinateClosureSeed struct {
	owner *keyspace.KeySpace
	key   keyspace.Key
}

func reverseOrderedCoordinateEqualityChain(t *testing.T, width int) ([]CoordinateKey, coordinateClosureSeed) {
	t.Helper()
	ks := keyspace.New()
	paths := make([]keyspace.Key, width+1)
	for index := range paths {
		path, ok := ks.FromResolverKey(symbol.ID(10000+index), 1, nil)
		if !ok {
			t.Fatalf("resolver path %d", index)
		}
		paths[index] = path
	}
	coordinates := make([]CoordinateKey, width)
	for index := 0; index < width; index++ {
		coordinate := BranchProofCoordinate(BranchProof{Kind: BranchProofPathEqual, Path: paths[index], Other: paths[index+1]})
		coordinates[width-index-1] = coordinate
	}
	return coordinates, coordinateClosureSeed{owner: ks, key: paths[0]}
}

func TestCoordinateFamilyRoundTripsAllFourCoupledMustCarriers(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	left, _, _ := coordinateTestLanes(t, reg, ks)
	skeleton, entries, ok := DecomposeCoordinates(left, ks)
	if !ok {
		t.Fatal("DecomposeCoordinates rejected a valid lane")
	}
	got, ok := ComposeCoordinates(skeleton, entries, reg, ks)
	if !ok {
		t.Fatal("ComposeCoordinates rejected its own canonical inventory")
	}
	if !Domain(reg).Equal(got, left) {
		t.Fatal("coordinate decomposition/composition changed PathEvidence semantics")
	}
	if len(entries) != 7 {
		t.Fatalf("coordinate count = %d, want 7 across all four must carriers", len(entries))
	}
}

func TestCoordinateFamilyJoinMeetAndWidenEqualCanonicalLaneLattice(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	left, right, _ := coordinateTestLanes(t, reg, ks)
	domain := Domain(reg)
	for _, test := range []struct {
		name     string
		skeleton func(CoordinateSkeleton, CoordinateSkeleton) CoordinateSkeleton
		scalar   func(*axis.Registry, CoordinateScalar, CoordinateScalar) CoordinateScalar
		want     Lane
	}{
		{name: "join", skeleton: CoordinateSkeletonJoin, scalar: CoordinateScalarJoin, want: domain.Join(left, right)},
		{name: "meet", skeleton: CoordinateSkeletonMeet, scalar: CoordinateScalarMeet, want: domain.Meet(left, right)},
		{name: "widen", skeleton: CoordinateSkeletonJoin, scalar: CoordinateScalarWiden, want: domain.Widen(left, right)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := coordinateBinaryLane(t, reg, ks, left, right, test.skeleton, test.scalar)
			if !domain.Equal(got, test.want) {
				t.Fatalf("factored %s differs from canonical PathEvidence lattice", test.name)
			}
		})
	}
}

func TestCoordinateFamilyBottomDefaultsPreserveFiniteFacts(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	finite, _, _ := coordinateTestLanes(t, reg, ks)
	bottom := Domain(reg).Bottom()
	for _, test := range []struct {
		name   string
		scalar func(*axis.Registry, CoordinateScalar, CoordinateScalar) CoordinateScalar
	}{
		{name: "join", scalar: CoordinateScalarJoin},
		{name: "widen", scalar: CoordinateScalarWiden},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := coordinateBinaryLane(t, reg, ks, bottom, finite, CoordinateSkeletonJoin, test.scalar)
			if !Domain(reg).Equal(got, finite) {
				t.Fatalf("factored Bottom %s finite lost a must coordinate", test.name)
			}
		})
	}
}

func TestCoordinateFamilyBoundaryApplyEqualsCanonicalLaneTransaction(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	destination, fragment, touched := coordinateTestLanes(t, reg, ks)
	dstSkeleton, dstEntries, ok := DecomposeCoordinates(destination, ks)
	if !ok {
		t.Fatal("destination decomposition failed")
	}
	fragSkeleton, fragEntries, ok := DecomposeCoordinates(fragment, ks)
	if !ok {
		t.Fatal("fragment decomposition failed")
	}
	dst := coordinateEntryMap(dstEntries)
	frag := coordinateEntryMap(fragEntries)
	keys := coordinateKeyUnion(dst, frag, ks)
	outEntries := make([]CoordinateEntry, 0, len(keys))
	touches := func(path keyspace.Key) bool { return path == touched }
	for _, key := range keys {
		dstScalar, present := dst[key]
		if !present {
			dstScalar = CoordinateDefault(dstSkeleton, key, reg)
		}
		fragScalar, present := frag[key]
		if !present {
			fragScalar = CoordinateDefault(fragSkeleton, key, reg)
		}
		scalar := ApplyCoordinateScalarBoundary(key, dstScalar, fragScalar, touches)
		if scalar.present {
			outEntries = append(outEntries, CoordinateEntry{Key: key, Scalar: scalar})
		}
	}
	skeleton := ApplyCoordinateSkeletonBoundary(dstSkeleton, fragSkeleton)
	skeleton, outEntries, ok = ApplyCoordinateRoots(skeleton, outEntries, reg, ks, nil, false)
	if !ok {
		t.Fatal("coordinate boundary normalization failed")
	}
	got, ok := ComposeCoordinates(skeleton, outEntries, reg, ks)
	if !ok {
		t.Fatal("coordinate boundary result did not compose")
	}
	want := destination.ApplyBoundary(ks, fragment, touches)
	if !Domain(reg).Equal(got, want) {
		t.Fatal("factored boundary replacement differs from canonical Lane.ApplyBoundary")
	}
}

func TestCoordinateFamilyRootWritesEqualCanonicalLaneTransaction(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	lane := Domain(reg).Bottom()
	path := mustStructKey(t, ks, pathdom.PathKey("sym77@1.result"))
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	skeleton, entries, ok := DecomposeCoordinates(lane, ks)
	if !ok {
		t.Fatal("bottom decomposition failed")
	}
	skeleton, entries, ok = ApplyCoordinateRoots(skeleton, entries, reg, ks, []CoordinateRoot{{Path: path, Value: value}}, true)
	if !ok {
		t.Fatal("coordinate root write failed")
	}
	got, ok := ComposeCoordinates(skeleton, entries, reg, ks)
	if !ok {
		t.Fatal("coordinate root result did not compose")
	}
	want, _ := lane.WritePathKey(reg, path, value)
	if !Domain(reg).Equal(got, want) {
		t.Fatal("factored root write differs from canonical Lane.WritePathKey")
	}
}

func TestCoordinateFamilyImportsEveryCoupledKeyAcrossKeyspaces(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	// Force opposite dense intern order before importing the semantic lane.
	_ = mustStructKey(t, from, pathdom.PathKey("sym90@1.alpha.deep"))
	_ = mustStructKey(t, to, pathdom.PathKey("sym91@1.zeta.deep"))
	lane, _, _ := coordinateTestLanes(t, reg, from)
	skeleton, entries, ok := DecomposeCoordinates(lane, from)
	if !ok {
		t.Fatal("source decomposition failed")
	}
	imported := make([]CoordinateEntry, len(entries))
	for index, entry := range entries {
		key, valid := ImportCoordinateKey(entry.Key, from, to, reg)
		if !valid {
			t.Fatalf("coordinate key %d failed import", index)
		}
		imported[index] = CoordinateEntry{Key: key, Scalar: entry.Scalar}
	}
	sort.Slice(imported, func(i, j int) bool { return CoordinateKeyLess(imported[i].Key, imported[j].Key, to) })
	got, ok := ComposeCoordinates(skeleton, imported, reg, to)
	if !ok {
		t.Fatal("imported coordinate inventory did not compose")
	}
	want, ok := lane.RekeyValueLanes(from, to)
	if !ok || !Domain(reg).Equal(got, want) {
		t.Fatal("coordinate import differs from canonical PathEvidence rekey")
	}
}

func coordinateTestLanes(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace) (Lane, Lane, keyspace.Key) {
	t.Helper()
	shared := mustStructKey(t, ks, pathdom.PathKey("sym1@1.shared"))
	leftOnly := mustStructKey(t, ks, pathdom.PathKey("sym1@1.left"))
	rightOnly := mustStructKey(t, ks, pathdom.PathKey("sym1@1.right"))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	commonProof := BranchProof{Kind: BranchProofPathPresence, Path: shared, Presence: presence.Present()}
	leftProof := BranchProof{Kind: BranchProofPathEqual, Path: shared, Other: leftOnly}
	rightProof := BranchProof{Kind: BranchProofPathEqual, Path: shared, Other: rightOnly}
	commonImplication := NewPathPresenceImplication(shared, presence.Present(), shared, presence.Present())
	leftImplication := NewPathValueRefinementImplication(shared, present, leftOnly, present)
	rightImplication := NewPathValueRefinementImplication(shared, absent, rightOnly, absent)

	left, _ := (Lane{}).WritePathKey(reg, shared, present)
	left, _ = left.WritePathKey(reg, leftOnly, present)
	left, _ = left.WritePathStaticMember(shared, present)
	left, _ = left.AddBranchProofs([]BranchProof{commonProof, leftProof})
	left, _ = left.AddPathPresenceImplication(commonImplication)
	left, _ = left.AddPathPresenceImplication(leftImplication)

	right, _ := (Lane{}).WritePathKey(reg, shared, absent)
	right, _ = right.WritePathKey(reg, rightOnly, absent)
	right, _ = right.WritePathStaticMember(shared, absent)
	right, _ = right.AddBranchProofs([]BranchProof{commonProof, rightProof})
	right, _ = right.AddPathPresenceImplication(commonImplication)
	right, _ = right.AddPathPresenceImplication(rightImplication)
	return left, right, shared
}

func coordinateBinaryLane(
	t *testing.T,
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	left, right Lane,
	skeletonOp func(CoordinateSkeleton, CoordinateSkeleton) CoordinateSkeleton,
	scalarOp func(*axis.Registry, CoordinateScalar, CoordinateScalar) CoordinateScalar,
) Lane {
	t.Helper()
	leftSkeleton, leftEntries, ok := DecomposeCoordinates(left, ks)
	if !ok {
		t.Fatal("left decomposition failed")
	}
	rightSkeleton, rightEntries, ok := DecomposeCoordinates(right, ks)
	if !ok {
		t.Fatal("right decomposition failed")
	}
	leftMap, rightMap := coordinateEntryMap(leftEntries), coordinateEntryMap(rightEntries)
	out := make([]CoordinateEntry, 0, len(leftMap)+len(rightMap))
	for _, key := range coordinateKeyUnion(leftMap, rightMap, ks) {
		leftScalar, present := leftMap[key]
		if !present {
			leftScalar = CoordinateDefault(leftSkeleton, key, reg)
		}
		rightScalar, present := rightMap[key]
		if !present {
			rightScalar = CoordinateDefault(rightSkeleton, key, reg)
		}
		scalar := scalarOp(reg, leftScalar, rightScalar)
		if scalar.present {
			out = append(out, CoordinateEntry{Key: key, Scalar: scalar})
		}
	}
	got, ok := ComposeCoordinates(skeletonOp(leftSkeleton, rightSkeleton), out, reg, ks)
	if !ok {
		t.Fatal("factored binary result did not compose")
	}
	return got
}

func coordinateEntryMap(entries []CoordinateEntry) map[CoordinateKey]CoordinateScalar {
	out := make(map[CoordinateKey]CoordinateScalar, len(entries))
	for _, entry := range entries {
		out[entry.Key] = entry.Scalar
	}
	return out
}

func coordinateKeyUnion(left, right map[CoordinateKey]CoordinateScalar, ks *keyspace.KeySpace) []CoordinateKey {
	set := make(map[CoordinateKey]struct{}, len(left)+len(right))
	for key := range left {
		set[key] = struct{}{}
	}
	for key := range right {
		set[key] = struct{}{}
	}
	out := make([]CoordinateKey, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return CoordinateKeyLess(out[i], out[j], ks) })
	return out
}
