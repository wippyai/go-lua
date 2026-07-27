package state

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func TestBoundaryPatchLaneTransposeEqualsWholeStateApply(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	lattice := domain.Lattice()

	source := Reachable(State{})
	for _, sample := range stateLawLaneSamples(reg, keys) {
		if sample.lane == LaneHeapTableIdentity {
			continue
		}
		source = lattice.Join(source, sample.state)
	}
	source = lattice.Join(source, boundaryPatchFactorHeapState(t, reg, keys))
	roots := boundaryArtifactCompleteLaneRoots(t, keys)
	artifact, err := ProjectBoundary(reg, keys, source, roots)
	if err != nil {
		t.Fatal(err)
	}
	destination := lattice.Bottom().WriteValue(reg, key.SymbolValue(60001), product.Top())
	want, err := ApplyBoundary(reg, keys, destination, artifact)
	if err != nil {
		t.Fatal(err)
	}

	patch, err := domain.SealBoundaryPatch(keys, artifact)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.Decompose(destination)
	if err != nil {
		t.Fatal(err)
	}
	for index := range factors {
		if factors[index].Lane().ID() == LaneValues {
			continue
		}
		switch factors[index].Lane().ID() {
		case LaneHeapTableIdentity:
			heap, heapErr := patch.HeapFactors()
			if heapErr != nil {
				t.Fatal(heapErr)
			}
			factors[index], err = heap.applyFactor(factors[index])
		case LanePlacement:
			placements, placementErr := patch.PlacementFactors()
			if placementErr != nil {
				t.Fatal(placementErr)
			}
			factors[index], err = placements.applyFactor(factors[index])
		default:
			factors[index], err = patch.ApplyLane(factors[index])
		}
		if err != nil {
			t.Fatalf("apply lane %q: %v", factors[index].Lane().ID(), err)
		}
	}
	residual, err := domain.Compose(factors)
	if err != nil {
		t.Fatal(err)
	}
	_, destinationValues := DecomposeValueLane(lattice, destination)
	values, err := patch.ApplyValues(destinationValues)
	if err != nil {
		t.Fatal(err)
	}
	got := RecomposeValueLane(reg, lattice, residual, values)
	if !lattice.Equal(got, want) {
		t.Fatal("lane-transposed boundary patch differs from canonical ApplyBoundary")
	}
}

func boundaryPatchFactorHeapState(t *testing.T, reg *axis.Registry, keys *keyspace.KeySpace) State {
	t.Helper()
	memberKey, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "boundary-factor"}})
	if !ok {
		t.Fatal("failed to intern boundary heap suffix")
	}
	id := identity.ID{Kind: "table", Site: "boundary-factor", Index: 1}
	return State{}.WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{memberKey: product.Absent(reg)},
	}))
}

func TestBoundaryPatchFailsClosedOnAuthorityAndCatalogDrift(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	artifact, err := ProjectBoundary(reg, keys, domain.Lattice().Bottom(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.SealBoundaryPatch(keyspace.New(), artifact); err == nil {
		t.Fatal("boundary patch accepted a foreign keyspace")
	}

	missing := domain
	missing.productDomainRuntime = new(productDomainRuntime)
	*missing.productDomainRuntime = *domain.productDomainRuntime
	missing.factorLanes = append([]productLaneRuntime(nil), domain.factorLanes...)
	missing.factorLanes[0].ops.boundaryApply = nil
	if _, err := missing.SealBoundaryPatch(keys, artifact); err == nil {
		t.Fatal("boundary patch accepted a missing catalog hook")
	}

	patch, err := domain.SealBoundaryPatch(keys, artifact)
	if err != nil {
		t.Fatal(err)
	}
	foreign := DefaultLaneCatalog().ProductDomain(reg)
	foreignFactors, err := foreign.Decompose(foreign.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := patch.ApplyLane(foreignFactors[1]); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign factor error = %v, want ErrInvalidLaneFactor", err)
	}
	valuesLane, ok := domain.ProductLane(LaneValues)
	if !ok {
		t.Fatal("default product has no Values lane")
	}
	valuesFactor, err := domain.LaneBottom(valuesLane)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := patch.ApplyLane(valuesFactor); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("Values lane error = %v, want ErrInvalidLaneFactor", err)
	}
	heapLane, ok := domain.ProductLane(LaneHeapTableIdentity)
	if !ok {
		t.Fatal("default product has no heap lane")
	}
	heapFactor, err := domain.LaneBottom(heapLane)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := patch.ApplyLane(heapFactor); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("HeapTableIdentity lane error = %v, want ErrInvalidLaneFactor", err)
	}
	placementLane, ok := domain.ProductLane(LanePlacement)
	if !ok {
		t.Fatal("default product has no placement lane")
	}
	placementFactor, err := domain.LaneBottom(placementLane)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := patch.ApplyLane(placementFactor); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("Placement lane error = %v, want ErrInvalidLaneFactor", err)
	}
}

func TestIdentityFactoredBoundaryPatchesEqualProjectedRebasedApply(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	domain := RegisteredProductDomain(reg)
	selected := identity.ID{Kind: "table", Site: "factored-boundary", Index: 1}
	outside := identity.ID{Kind: "table", Site: "factored-boundary", Index: 2}
	suffix := []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}
	fromMember, ok := from.FromRootlessSuffix(suffix)
	if !ok {
		t.Fatal("failed to intern source member suffix")
	}
	toMember, ok := to.FromRootlessSuffix(suffix)
	if !ok {
		t.Fatal("failed to intern destination member suffix")
	}
	root := identityvalue.Present(reg, selected)
	source := domain.Lattice().Bottom().
		WriteHeapTableObject(reg, selected, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: root, StaticMembers: map[keyspace.Key]product.Value{fromMember: product.Absent(reg)},
		})).
		WritePlacement(selected, placement.SharedHeap)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Value: root}})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to,
		BoundaryRootMap{{FromRoot: 0, ToRoot: 0}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	destination := domain.Lattice().Bottom().
		WriteHeapTableObject(reg, selected, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{toMember: product.Top()},
		})).
		WriteHeapTableObject(reg, outside, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)})).
		WritePlacement(selected, placement.Stack).
		WritePlacement(outside, placement.OwnedHeap)
	want, err := ApplyBoundary(reg, to, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := domain.SealBoundaryPatch(to, rebased)
	if err != nil {
		t.Fatal(err)
	}

	destinationHeap := onlyHeapTableIdentityFactor(t, domain, destination)
	heapPatch, err := patch.HeapFactors()
	if err != nil {
		t.Fatal(err)
	}
	gotHeap, err := heapPatch.applyFactor(destinationHeap)
	if err != nil {
		t.Fatal(err)
	}
	wantHeap := onlyHeapTableIdentityFactor(t, domain, want)
	equal, err := domain.LaneEqual(gotHeap, wantHeap)
	if err != nil || !equal {
		t.Fatalf("factored heap apply equal=%t err=%v", equal, err)
	}

	destinationPlacement := onlyPlacementLaneFactor(t, domain, destination)
	placementPatch, err := patch.PlacementFactors()
	if err != nil {
		t.Fatal(err)
	}
	gotPlacement, err := placementPatch.applyFactor(destinationPlacement)
	if err != nil {
		t.Fatal(err)
	}
	wantPlacement := onlyPlacementLaneFactor(t, domain, want)
	equal, err = domain.LaneEqual(gotPlacement, wantPlacement)
	if err != nil || !equal {
		t.Fatalf("factored placement apply equal=%t err=%v", equal, err)
	}
}
