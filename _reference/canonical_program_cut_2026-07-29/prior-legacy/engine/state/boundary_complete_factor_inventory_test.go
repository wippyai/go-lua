package state

import (
	"context"
	"strings"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// The formal Apply executor transports the complete registered factor tuple.
// Coordinate-factored lanes are deliberately represented by their ordinary
// LaneFactor carrier here: Apply must dispatch the lane's already-frozen laws,
// never rediscover or special-case its coordinate family.
func TestBoundaryFactorPatchCoversCompleteRegisteredFactorInventory(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	fromPath := from.FromPath(pathdom.Path{Root: "complete-factor-source"})
	toPath := to.FromPath(pathdom.Path{Root: "complete-factor-target"})
	fromSlot, toSlot := statekey.SymbolValue(9801), statekey.SymbolValue(9802)

	authority, err := NewBoundaryAllocationAuthority(
		RootBoundaryAllocationRoute(lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: toPath, ToSlot: toSlot}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	companionLane, ok := domain.BoundaryClosureCompanion()
	if !ok {
		t.Fatal("registered product has no closure companion")
	}

	want := map[LaneID]bool{
		LanePathEvidence:      false,
		LaneNumFloors:         false,
		LaneNumCeils:          false,
		LaneLenFloors:         false,
		LaneDiffRelations:     false,
		LaneHeapTableIdentity: false,
		LanePlacement:         false,
	}
	samples := stateLawLaneSamples(reg, from)
	joined := domain.Lattice().Bottom()
	for _, sample := range samples {
		joined = domain.Lattice().Join(joined, sample.state)
	}
	samples = append(samples, stateLawLaneSample{lane: "all-registered-lanes", state: joined})
	for _, sample := range samples {
		if sample.lane == LaneValues {
			continue
		}
		source := domain.Normalize(sample.state.WriteValue(reg, fromSlot, product.Top()))
		destination := domain.Lattice().Bottom().WriteValue(reg, toSlot, product.Absent(reg))
		selection, err := SealBoundaryFactorSelection(from, []BoundaryFactorRoot{{Slot: fromSlot, Path: fromPath}}, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		sourceAll, err := domain.Decompose(source)
		if err != nil {
			t.Fatal(err)
		}
		var companionFactor *LaneFactor
		for index := range sourceAll {
			if sourceAll[index].Lane() == companionLane {
				companionFactor = &sourceAll[index]
				break
			}
		}
		companion, err := domain.ProjectBoundaryClosureCompanion(selection, companionFactor)
		if err != nil {
			t.Fatalf("project companion for %q: %v", sample.lane, err)
		}
		plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
		if err != nil {
			t.Fatalf("prepare plan for %q: %v", sample.lane, err)
		}
		sourceResidual, sourceValues := DecomposeValueLane(domain.Lattice(), source)
		destinationResidual, destinationValues := DecomposeValueLane(domain.Lattice(), destination)
		sourceFactors, err := domain.DecomposeLanes(sourceResidual, domain.NonValuesLaneInventory())
		if err != nil {
			t.Fatal(err)
		}
		destinationFactors, err := domain.DecomposeLanes(destinationResidual, domain.NonValuesLaneInventory())
		if err != nil {
			t.Fatal(err)
		}
		factored, err := ApplyBoundaryFactorTuple(
			context.Background(), plan, plan.values,
			BoundaryFactorTuple[statekey.Value]{Values: destinationValues, Factors: destinationFactors},
			BoundaryFactorTuple[statekey.Value]{Values: sourceValues, Factors: sourceFactors},
			[]product.Value{product.Top()}, []BoundaryFactorRootTarget[statekey.Value]{{Slot: toSlot, WriteScalar: true}},
		)
		if err != nil {
			t.Fatalf("factor transaction for %q: %v", sample.lane, err)
		}
		gotResidual, err := domain.ComposeSparse(factored.Factors)
		if err != nil {
			t.Fatal(err)
		}
		got := RecomposeValueLane(reg, domain.Lattice(), gotResidual, factored.Values)

		artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Slot: fromSlot, Path: fromPath, Value: product.Top()}})
		if err != nil {
			t.Fatalf("canonical project for %q: %v", sample.lane, err)
		}
		rebased, err := transport.Rebase(reg, artifact)
		if err != nil {
			t.Fatalf("canonical rebase for %q: %v", sample.lane, err)
		}
		canonical, err := ApplyBoundary(reg, to, destination, rebased)
		if err != nil {
			t.Fatalf("canonical apply for %q: %v", sample.lane, err)
		}
		if !domain.Lattice().Equal(got, canonical) {
			t.Fatalf("factor transaction differs from canonical whole-State boundary for %q", sample.lane)
		}
		if _, tracked := want[sample.lane]; tracked {
			want[sample.lane] = true
		}
	}
	for lane, covered := range want {
		if !covered {
			t.Errorf("complete factor inventory did not cover %q", lane)
		}
	}
}

func TestBoundaryFactorTupleRejectsValuesTransportFromAnotherPlan(t *testing.T) {
	plan, _, _ := boundaryGenericValueTestPlan(t)
	other := plan
	other.seal = new(boundaryFactorTransportSeal)
	relation, err := SealBoundaryValueSlotRelation[statekey.Value, statekey.Value](nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	foreignValues, err := PrepareBoundaryValueFactorTransport(other, relation)
	if err != nil {
		t.Fatal(err)
	}

	destinationState := plan.domain.Lattice().Bottom()
	destinationResidual, destinationValues := DecomposeValueLane(plan.domain.Lattice(), destinationState)
	destinationFactors, err := plan.domain.DecomposeLanes(destinationResidual, plan.domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	sourceState := plan.sourceDomain.Lattice().Bottom()
	sourceResidual, sourceValues := DecomposeValueLane(plan.sourceDomain.Lattice(), sourceState)
	sourceFactors, err := plan.sourceDomain.DecomposeLanes(sourceResidual, plan.sourceDomain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyBoundaryFactorTuple(
		context.Background(), plan, foreignValues,
		BoundaryFactorTuple[statekey.Value]{Values: destinationValues, Factors: destinationFactors},
		BoundaryFactorTuple[statekey.Value]{Values: sourceValues, Factors: sourceFactors},
		nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "boundary factor tuple is unowned") {
		t.Fatalf("cross-plan Values transport error=%v, want plan-ownership rejection", err)
	}
}
