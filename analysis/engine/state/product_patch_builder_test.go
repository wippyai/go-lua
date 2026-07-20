package state

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestProductPatchBuilderExtractsOnlySealedFragmentWrites(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	placementLane, ok := domain.ProductLane(LanePlacement)
	if !ok {
		t.Fatal("missing placement lane")
	}
	slot := key.SymbolValue(91)
	plan, err := domain.SealProductPatch(
		[]ProductLane{placementLane}, []ProductLane{placementLane},
		nil, false, []key.Value{slot}, false, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	id := identity.ID{Kind: "table", Site: "product-patch", Index: 1}
	base := domain.Lattice().Bottom().WritePlacement(id, placement.Stack).WriteValue(reg, slot, product.Top())
	carry, err := domain.DecomposeLanes(base, []ProductLane{placementLane})
	if err != nil {
		t.Fatal(err)
	}
	_, carryValues := DecomposeValueLane(domain.Lattice(), base)
	builder, err := plan.NewBuilder(carry, carryValues, true)
	if err != nil {
		t.Fatal(err)
	}
	fragment := domain.Lattice().Bottom().WritePlacement(id, placement.OwnedHeap).WriteValue(reg, slot, domain.ValueBottom())
	if err := builder.WriteDeclaredFragment(fragment, true); err != nil {
		t.Fatal(err)
	}
	factors, values, reachable, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	residual, err := domain.ComposeSparse(factors)
	if err != nil {
		t.Fatal(err)
	}
	got := RecomposeValueLane(reg, domain.Lattice(), residual, values)
	if !reachable || got.ReadPlacement(id) != placement.OwnedHeap || !product.Equal(reg, got.ReadValue(reg, slot), domain.ValueBottom()) {
		t.Fatalf("sealed fragment extraction: reachable=%t placement=%v valueBottom=%t", reachable, got.ReadPlacement(id), product.Equal(reg, got.ReadValue(reg, slot), domain.ValueBottom()))
	}
}

func TestProductPatchBuilderRejectsUndeclaredFragmentFacts(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	placementLane, ok := domain.ProductLane(LanePlacement)
	if !ok {
		t.Fatal("missing placement lane")
	}
	declaredSlot := key.SymbolValue(92)
	plan, err := domain.SealProductPatch(nil, []ProductLane{placementLane}, nil, false, []key.Value{declaredSlot}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	newBuilder := func() *ProductPatchBuilder {
		builder, buildErr := plan.NewBuilder(nil, ValueLaneFactor{}, false)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		return builder
	}
	declaredID := identity.ID{Kind: "table", Site: "declared-placement", Index: 1}
	undeclaredID := identity.ID{Kind: "table", Site: "undeclared-frozen", Index: 2}
	fragment := domain.Lattice().Bottom().WritePlacement(declaredID, placement.Stack).FreezeTable(undeclaredID)
	if err := newBuilder().WriteDeclaredFragment(fragment, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("undeclared lane error = %v", err)
	}

	extraSlot := key.SymbolValue(93)
	fragment = domain.Lattice().Bottom().WritePlacement(declaredID, placement.Stack).WriteValue(reg, extraSlot, product.Top())
	if err := newBuilder().WriteDeclaredFragment(fragment, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("undeclared Values slot error = %v", err)
	}

	fragment = domain.Lattice().Top()
	if err := newBuilder().WriteDeclaredFragment(fragment, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("undeclared Values Top error = %v", err)
	}
	valuesOwner, err := domain.SealProductPatch(nil, nil, nil, false, nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	ownerBuilder, err := valuesOwner.NewBuilder(nil, ValueLaneFactor{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownerBuilder.WriteDeclaredFragment(domain.Lattice().Bottom().WriteValue(reg, extraSlot, product.Top()), true); err != nil {
		t.Fatalf("Values owner rejected finite slot: %v", err)
	}
}

func TestProductPatchBuilderAllowsOnlyExactSealedCarryCopies(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	pathLane, ok := domain.ProductLane(LanePathEvidence)
	if !ok {
		t.Fatal("missing path-evidence lane")
	}
	path, ok := keys.FromStateKey("product-patch-carry")
	if !ok {
		t.Fatal("invalid carry path")
	}
	slot := key.SymbolValue(94)
	base := domain.Lattice().Bottom().WriteLocalPathKey(reg, path, product.Top()).WriteValue(reg, slot, product.Top())
	carryFactors, err := domain.DecomposeLanes(base, []ProductLane{pathLane})
	if err != nil {
		t.Fatal(err)
	}
	_, carryValues := DecomposeValueLane(domain.Lattice(), base)
	plan, err := domain.SealProductPatch([]ProductLane{pathLane}, nil, []key.Value{slot}, false, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	newBuilder := func() *ProductPatchBuilder {
		builder, buildErr := plan.NewBuilder(carryFactors, carryValues, true)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		return builder
	}
	if err := newBuilder().WriteDeclaredFragment(base, true); err != nil {
		t.Fatalf("exact carried fragment rejected: %v", err)
	}
	changedLane := base.WriteLocalPathKey(reg, path, presentValue(reg))
	if err := newBuilder().WriteDeclaredFragment(changedLane, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("changed carried lane error = %v", err)
	}
	changedValue := base.WriteValue(reg, slot, presentValue(reg))
	if err := newBuilder().WriteDeclaredFragment(changedValue, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("changed carried Values slot error = %v", err)
	}
	// Product Bottom erases every axis when reachability becomes false. A
	// reachability-owning transaction must publish only that terminal bit,
	// without misclassifying the erased carried lane and Values slot as writes.
	terminated := newBuilder()
	if err := terminated.WriteDeclaredFragment(domain.Lattice().Bottom(), false); err != nil {
		t.Fatalf("unreachable transaction rejected arbitrary carried axes: %v", err)
	}
	_, _, reachable, err := terminated.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if reachable {
		t.Fatal("unreachable transaction published reachable=true")
	}
	noReachability, err := domain.SealProductPatch([]ProductLane{pathLane}, nil, []key.Value{slot}, false, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	withoutOwnership, err := noReachability.NewBuilder(carryFactors, carryValues, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := withoutOwnership.WriteDeclaredFragment(domain.Lattice().Bottom(), false); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("unreachable transaction without reachability ownership error = %v", err)
	}
}

func TestProductPatchBuilderCarriesWholeValuesButOwnsOnlyFiniteWrites(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	declared := key.SymbolValue(95)
	untouched := key.SymbolValue(96)
	plan, err := domain.SealProductPatch(nil, nil, nil, true, []key.Value{declared}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	carry := ValueLaneFactor{Values: map[key.Value]product.Value{
		declared:  product.Top(),
		untouched: presentValue(reg),
	}}
	newBuilder := func() *ProductPatchBuilder {
		builder, buildErr := plan.NewBuilder(nil, carry, true)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		return builder
	}
	exact := domain.Lattice().Bottom().
		WriteValue(reg, declared, domain.ValueBottom()).
		WriteValue(reg, untouched, presentValue(reg))
	if err := newBuilder().WriteDeclaredFragment(exact, true); err != nil {
		t.Fatalf("whole Values carry rejected an unchanged undeclared slot: %v", err)
	}
	changed := exact.WriteValue(reg, untouched, product.Top())
	if err := newBuilder().WriteDeclaredFragment(changed, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("whole Values carry accepted changed undeclared slot: %v", err)
	}
	deleted := domain.Lattice().Bottom().WriteValue(reg, declared, domain.ValueBottom())
	if err := newBuilder().WriteDeclaredFragment(deleted, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("whole Values carry accepted deleted undeclared slot: %v", err)
	}
	if err := newBuilder().WriteDeclaredFragment(domain.Lattice().Top(), true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("whole Values carry accepted finite-to-Top change: %v", err)
	}
	topBuilder, err := plan.NewBuilder(nil, ValueLaneFactor{Top: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := topBuilder.WriteDeclaredFragment(exact, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("whole Values carry accepted Top-to-finite change: %v", err)
	}
	if _, err := domain.SealProductPatch(nil, nil, []key.Value{untouched}, true, nil, false, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("whole Values carry accepted finite carry overlap: %v", err)
	}
	if _, err := domain.SealProductPatch(nil, nil, nil, true, nil, true, true); !errors.Is(err, ErrInvalidProductPatch) {
		t.Fatalf("whole Values carry accepted whole write overlap: %v", err)
	}
}

func TestProductPatchBuilderBoundaryEqualsCanonicalApply(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	source := Reachable(State{})
	for _, sample := range stateLawLaneSamples(reg, keys) {
		if sample.lane == LaneHeapTableIdentity {
			continue
		}
		source = domain.Lattice().Join(source, sample.state)
	}
	source = domain.Lattice().Join(source, boundaryPatchFactorHeapState(t, reg, keys))
	artifact, err := ProjectBoundary(reg, keys, source, boundaryArtifactCompleteLaneRoots(t, keys))
	if err != nil {
		t.Fatal(err)
	}
	patch, err := domain.SealBoundaryPatch(keys, artifact)
	if err != nil {
		t.Fatal(err)
	}
	destination := domain.Lattice().Bottom().WriteValue(reg, key.SymbolValue(60001), product.Top())
	want, err := ApplyBoundary(reg, keys, destination, artifact)
	if err != nil {
		t.Fatal(err)
	}
	lanes := domain.NonValuesLaneInventory()
	plan, err := domain.SealProductPatch(lanes, lanes, nil, false, nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	carry, err := domain.DecomposeLanes(destination, lanes)
	if err != nil {
		t.Fatal(err)
	}
	_, carryValues := DecomposeValueLane(domain.Lattice(), destination)
	builder, err := plan.NewBuilder(carry, carryValues, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.ApplyBoundary(patch); err != nil {
		t.Fatal(err)
	}
	factors, values, reachable, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	residual, err := domain.ComposeSparse(factors)
	if err != nil {
		t.Fatal(err)
	}
	got := RecomposeValueLane(reg, domain.Lattice(), residual, values)
	if !reachable || !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-only builder boundary transaction differs from canonical ApplyBoundary")
	}
}
