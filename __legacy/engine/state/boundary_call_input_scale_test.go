package state

import (
	"fmt"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestBoundaryCallInputFactorsScaleByRegisteredCoordinates(t *testing.T) {
	const (
		valueWidth         = 66
		pathScalarWidth    = 67
		unrelatedPathWidth = 128
		ordinaryWidth      = 9
	)
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	roots := make([]BoundaryFactorRoot, 0, valueWidth+pathScalarWidth)
	rootMap := make(BoundaryRootMap, 0, valueWidth+pathScalarWidth)
	sourceValueSlots := make([]statekey.Value, valueWidth)
	targetValueSlots := make([]statekey.Value, valueWidth)
	for index := 0; index < valueWidth; index++ {
		sourceValueSlots[index] = statekey.SymbolValue(symbol.ID(8100 + index))
		targetValueSlots[index] = statekey.SymbolValue(symbol.ID(9100 + index))
		root := len(roots)
		roots = append(roots, BoundaryFactorRoot{Slot: sourceValueSlots[index]})
		rootMap = append(rootMap, BoundaryRootBinding{FromRoot: root, ToRoot: root, ToSlot: targetValueSlots[index]})
	}

	pathLane, ok := domain.ProductLane(LanePathEvidence)
	if !ok {
		t.Fatal("registered product has no PathEvidence lane")
	}
	families, err := domain.CoordinateFamilies(pathLane)
	if err != nil || len(families) != 1 {
		t.Fatalf("PathEvidence coordinate families = %d/%v, want one", len(families), err)
	}
	sourcePathState := domain.Lattice().Bottom()
	for index := 0; index < pathScalarWidth; index++ {
		sourcePath := from.FromPath(pathdom.Path{Root: fmt.Sprintf("call-input-source-%d", index)})
		targetPath := to.FromPath(pathdom.Path{Root: fmt.Sprintf("call-input-target-%d", index)})
		sourcePathState = sourcePathState.WriteLocalPathKey(reg, sourcePath, typevalue.String(reg))
		root := len(roots)
		roots = append(roots, BoundaryFactorRoot{Path: sourcePath})
		rootMap = append(rootMap, BoundaryRootBinding{FromRoot: root, ToRoot: root, To: targetPath})
	}
	// These coordinates share the same registered family/lane but are outside
	// every boundary root fiber. They must remain semantic state without becoming
	// physical operands of this call transport.
	for index := 0; index < unrelatedPathWidth; index++ {
		unrelated := from.FromPath(pathdom.Path{Root: fmt.Sprintf("call-input-unrelated-source-%d", index)})
		sourcePathState = sourcePathState.WriteLocalPathKey(reg, unrelated, typevalue.String(reg))
	}
	pathFactors, err := domain.DecomposeLanes(sourcePathState, []ProductLane{pathLane})
	if err != nil {
		t.Fatal(err)
	}
	sourceSkeleton, sourceScalars, err := domain.DecomposeCoordinateFamily(pathFactors[0], families[0], from)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceScalars) != pathScalarWidth+unrelatedPathWidth {
		t.Fatalf("PathEvidence scalar width = %d, want %d", len(sourceScalars), pathScalarWidth+unrelatedPathWidth)
	}
	sourceCoordinateSlots := make([]CoordinateSlot, len(sourceScalars))
	for index := range sourceScalars {
		sourceCoordinateSlots[index] = sourceScalars[index].Slot()
	}

	selection, err := SealBoundaryFactorSelection(from, roots, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	selection, err = domain.ExpandBoundaryFactorCoordinateClosure(selection, sourceCoordinateSlots)
	if err != nil {
		t.Fatal(err)
	}
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(lexicalidentity.RootBody(namespace)), nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, rootMap, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	companionLane, ok := domain.BoundaryClosureCompanion()
	if !ok {
		t.Fatal("registered product has no boundary closure companion")
	}
	companionFactor, err := domain.LaneBottom(companionLane)
	if err != nil {
		t.Fatal(err)
	}
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, &companionFactor)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}
	affectedSource, err := plan.CoordinateSourceFiberIndexes(families[0], sourceCoordinateSlots)
	if err != nil {
		t.Fatal(err)
	}
	if len(affectedSource) != pathScalarWidth {
		t.Fatalf("affected source coordinate width = %d, want %d (from %d registered coordinates)", len(affectedSource), pathScalarWidth, len(sourceCoordinateSlots))
	}

	ordinary := 0
	for _, lane := range domain.NonValuesLaneInventory() {
		families, familyErr := domain.CoordinateFamilies(lane)
		if familyErr != nil {
			t.Fatal(familyErr)
		}
		if len(families) != 0 {
			continue
		}
		factor, factorErr := domain.LaneBottom(lane)
		if factorErr != nil {
			t.Fatal(factorErr)
		}
		patch, patchErr := plan.PrepareLane(factor, true)
		if patchErr != nil {
			t.Fatalf("prepare ordinary lane %q: %v", lane.ID(), patchErr)
		}
		if _, applyErr := patch.ApplyLane(factor); applyErr != nil {
			t.Fatalf("apply ordinary lane %q: %v", lane.ID(), applyErr)
		}
		ordinary++
	}
	if ordinary != ordinaryWidth {
		t.Fatalf("independent ordinary lane width = %d, want %d", ordinary, ordinaryWidth)
	}
	if _, err := plan.PrepareLane(pathFactors[0], true); err == nil {
		t.Fatal("ordinary boundary lane adapter accepted a registered coordinate-family lane")
	}
	rootValues := make([]product.Value, len(roots))
	for index := range rootValues {
		rootValues[index] = product.Bottom(reg)
		if roots[index].Path.Kind != keyspace.KindInvalid {
			rootValues[index] = typevalue.String(reg)
		}
	}
	if _, err := plan.PrepareFactor(pathFactors[0], rootValues, true); err == nil {
		t.Fatal("ordinary boundary factor adapter accepted a registered coordinate-family lane")
	}

	valueContributions := 0
	for index, slot := range sourceValueSlots {
		contributions, contributionErr := plan.RebaseValueSlot(slot, typevalue.String(reg))
		if contributionErr != nil {
			t.Fatal(contributionErr)
		}
		if len(contributions) != 1 || contributions[0].Slot != targetValueSlots[index] {
			t.Fatalf("Values source %d contributions = %#v, want one exact target", index, contributions)
		}
		valueContributions += len(contributions)
	}
	if valueContributions != valueWidth {
		t.Fatalf("Values contribution width = %d, want %d", valueContributions, valueWidth)
	}

	unrelatedPath := to.FromPath(pathdom.Path{Root: "call-input-unrelated-destination"})
	unrelatedState := domain.Lattice().Bottom().WriteLocalPathKey(reg, unrelatedPath, typevalue.String(reg))
	unrelatedFactors, err := domain.DecomposeLanes(unrelatedState, []ProductLane{pathLane})
	if err != nil {
		t.Fatal(err)
	}
	destinationSkeleton, unrelatedScalars, err := domain.DecomposeCoordinateFamily(unrelatedFactors[0], families[0], to)
	if err != nil || len(unrelatedScalars) != 1 {
		t.Fatalf("unrelated destination coordinate = %d/%v, want one", len(unrelatedScalars), err)
	}
	sourceShape, err := domain.SealCoordinateFamilyShape(sourceSkeleton, sourceCoordinateSlots)
	if err != nil {
		t.Fatal(err)
	}
	destinationShape, err := domain.SealCoordinateFamilyShape(destinationSkeleton, coordinateSlots(unrelatedScalars))
	if err != nil {
		t.Fatal(err)
	}
	lift, err := plan.PrepareCoordinateBoundaryFamilyLift(sourceShape, destinationShape, true)
	if err != nil {
		t.Fatal(err)
	}
	if lift.WireCount() != pathScalarWidth {
		t.Fatalf("coordinate wire width = %d, want %d", lift.WireCount(), pathScalarWidth)
	}
	coordinateOperands := 0
	maxLiveDependencies := 0
	for wireIndex := 0; wireIndex < lift.WireCount(); wireIndex++ {
		wire := lift.Wire(wireIndex)
		live := wire.SourceCount(lift) + wire.RootCount(lift)
		if live > maxLiveDependencies {
			maxLiveDependencies = live
		}
		coordinateOperands += live
	}
	if coordinateOperands != pathScalarWidth || maxLiveDependencies != 1 {
		t.Fatalf("coordinate operands/max live dependencies = %d/%d, want %d/1", coordinateOperands, maxLiveDependencies, pathScalarWidth)
	}
	var visited int
	allocations := testing.AllocsPerRun(1000, func() {
		visited = 0
		for wireIndex := 0; wireIndex < lift.WireCount(); wireIndex++ {
			wire := lift.Wire(wireIndex)
			for index := 0; index < wire.SourceCount(lift); index++ {
				if _, ok := wire.SourceIndex(lift, index); !ok {
					t.Fatal("sealed source operand disappeared")
				}
				visited++
			}
			for index := 0; index < wire.RootCount(lift); index++ {
				if _, ok := wire.RootIndex(lift, index); !ok {
					t.Fatal("sealed root operand disappeared")
				}
				visited++
			}
		}
	})
	if allocations != 0 || visited != pathScalarWidth {
		t.Fatalf("sealed affected-fiber walk allocations/width = %.0f/%d, want 0/%d", allocations, visited, pathScalarWidth)
	}
	if got, want := valueContributions+ordinary+coordinateOperands, valueWidth+ordinaryWidth+pathScalarWidth; got != want {
		t.Fatalf("factored boundary work width = %d, want additive %d", got, want)
	}
	liftRootValues := make([]product.Value, len(roots))
	for index := range liftRootValues {
		liftRootValues[index] = typevalue.String(reg)
	}
	got, err := lift.Apply(unrelatedFactors[0], pathFactors[0], liftRootValues)
	if err != nil {
		t.Fatal(err)
	}
	_, gotScalars, err := domain.DecomposeCoordinateFamily(got, families[0], to)
	if err != nil || len(gotScalars) != pathScalarWidth+1 {
		t.Fatalf("lift output scalar width = %d/%v, want %d", len(gotScalars), err, pathScalarWidth+1)
	}
}
