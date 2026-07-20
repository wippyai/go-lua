package state

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestValueLaneFactorExactInverseNormalizesOnlyNoncanonicalInput(t *testing.T) {
	reg := standard.Registry()
	productDomain := RegisteredProductDomain(reg)
	domain := productDomain.Lattice()
	original := Reachable(State{}).WriteValue(reg, statekey.SymbolValue(7), product.Top())
	want := NormalizeForDomain(domain, original)

	// Exercise the defensive public boundary: a noncanonical spelling is
	// normalized once before decomposition, while recomposition of its exact
	// factors does not rejoin unrelated lanes.
	original.canonical = false
	residual, values := DecomposeValueLane(domain, original)
	got := RecomposeValueLane(reg, domain, residual, values)
	if !got.canonical || !domain.Equal(got, want) {
		t.Fatalf("Value-lane inverse lost canonical semantics: canonical=%t", got.canonical)
	}
}

func TestProductDomainNormalizeUsesExactScopeIdentity(t *testing.T) {
	reg := standard.Registry()
	full := RegisteredProductDomain(reg)
	value := full.Normalize(Reachable(State{}).WriteValue(reg, statekey.SymbolValue(9), product.Top()))
	before, err := full.Decompose(value)
	if err != nil {
		t.Fatal(err)
	}
	after, err := full.Decompose(full.Normalize(value))
	if err != nil {
		t.Fatal(err)
	}
	heapIndex := -1
	for index := range before {
		if before[index].Lane().ID() == LaneHeapTableIdentity {
			heapIndex = index
			break
		}
	}
	if heapIndex < 0 {
		t.Fatal("registered product has no heap lane")
	}
	if same, sameErr := full.LaneSame(before[heapIndex], after[heapIndex]); sameErr != nil || !same {
		t.Fatalf("canonical same-scope normalization lost heap representation identity: same=%t err=%v", same, sameErr)
	}

	valuesOnly, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues})
	if err != nil {
		t.Fatal(err)
	}
	foreignScope := value.WritePlacement(identity.ID{Kind: "table", Site: "normalize-scope", Index: 1}, placement.Stack)
	got := valuesOnly.Normalize(foreignScope)
	if got.laneMask != valuesOnly.mask || got.ReadPlacement(identity.ID{Kind: "table", Site: "normalize-scope", Index: 1}) != placement.Bottom {
		t.Fatal("canonical foreign-scope value bypassed defensive normalization")
	}

	noncanonical := value
	noncanonical.canonical = false
	if got := full.Normalize(noncanonical); !got.canonical || !full.Lattice().Equal(got, value) {
		t.Fatal("noncanonical same-scope value bypassed defensive normalization")
	}
}

func TestSealedFactorSelectionProjectsAndPatchesWithoutAllocation(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	selection, err := domain.SealFactorSelection(NewLaneSet(LanePlacement))
	if err != nil {
		t.Fatal(err)
	}
	id := identity.ID{Kind: "table", Site: "sealed-factor-selection", Index: 1}
	slot := statekey.SymbolValue(17)
	base := domain.Lattice().Bottom().
		WriteValue(reg, slot, product.Top()).
		WritePlacement(id, placement.Stack)
	delta := domain.Lattice().Bottom().WritePlacement(id, placement.OwnedHeap)

	projected, err := domain.ProjectSelectedFactors(base, selection)
	if err != nil {
		t.Fatal(err)
	}
	if got := projected.ReadPlacement(id); got != placement.Stack {
		t.Fatalf("projected placement = %v, want stack", got)
	}
	if got := projected.ReadValue(reg, slot); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("whole-lane projection retained unselected Values")
	}

	patched, err := domain.PatchSelectedFactors(base, delta, selection)
	if err != nil {
		t.Fatal(err)
	}
	if got := patched.ReadPlacement(id); got != placement.OwnedHeap {
		t.Fatalf("patched placement = %v, want heap", got)
	}
	if got := patched.ReadValue(reg, slot); !product.Equal(reg, got, product.Top()) {
		t.Fatal("factor patch changed unselected Values")
	}

	if allocs := testing.AllocsPerRun(1000, func() {
		if _, projectErr := domain.ProjectSelectedFactors(base, selection); projectErr != nil {
			panic(projectErr)
		}
	}); allocs > 1 {
		t.Fatalf("sealed factor projection allocations = %.1f, want at most the escaped State result", allocs)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, patchErr := domain.PatchSelectedFactors(base, delta, selection); patchErr != nil {
			panic(patchErr)
		}
	}); allocs > 1 {
		t.Fatalf("sealed factor patch allocations = %.1f, want at most the escaped State result", allocs)
	}
}

func TestSealedFactorSelectionRejectsValuesAndForeignDomain(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	if _, err := domain.SealFactorSelection(NewLaneSet(LaneValues)); !errors.Is(err, ErrInvalidProductLane) {
		t.Fatalf("Values selection error = %v, want ErrInvalidProductLane", err)
	}
	selection, err := domain.SealFactorSelection(NewLaneSet(LanePlacement))
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := TryRegisteredProductDomainWithLanes(reg, DefaultLanes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.ProjectSelectedFactors(foreign.Lattice().Bottom(), selection); !errors.Is(err, ErrInvalidProductLane) {
		t.Fatalf("foreign selection error = %v, want ErrInvalidProductLane", err)
	}
}

func TestLaneMaskInlineFastPathAndCanonicalUnboundedSpill(t *testing.T) {
	inlineBits := make([]laneBit, len(DefaultLanes()))
	for i := range inlineBits {
		inlineBits[i] = laneBit(i)
	}
	inline := scopedLaneMask(inlineBits)
	if inline.spill != laneMaskScopeMarker {
		t.Fatalf("default %d-lane mask used spill storage", len(inlineBits))
	}
	if allocs := testing.AllocsPerRun(1000, func() { _ = scopedLaneMask(inlineBits) }); allocs != 0 {
		t.Fatalf("inline lane mask allocations = %g, want 0", allocs)
	}

	left := scopedLaneMask([]laneBit{129, 0, 64, 65, 127})
	right := scopedLaneMask([]laneBit{65, 127, 64, 0, 129})
	if left != right {
		t.Fatalf("equal unbounded masks differ: %#v != %#v", left, right)
	}
	if left.hash64() != right.hash64() {
		t.Fatalf("equal unbounded masks hash differently: %d != %d", left.hash64(), right.hash64())
	}
	var got []laneBit
	left.forEach(func(bit laneBit) bool {
		got = append(got, bit)
		return true
	})
	want := []laneBit{0, 64, 65, 127, 129}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lane-mask iteration = %v, want %v", got, want)
	}
	for _, bit := range want {
		if !left.allows(bit) {
			t.Errorf("mask does not allow selected lane %d", bit)
		}
	}
	for _, bit := range []laneBit{1, 63, 66, 128, 130, 1024} {
		if left.allows(bit) {
			t.Errorf("mask allows unselected lane %d", bit)
		}
	}
}

func TestLaneCatalogAndProductFactorsHaveNoMachineWordCap(t *testing.T) {
	reg := standard.Registry()
	const laneCount = 130
	specs := make([]laneSpec, laneCount)
	ids := make([]LaneID, laneCount)
	for i := range specs {
		specs[i] = valuesLaneSpec
		specs[i].slotFactored = false
		specs[i].formalRekey = formalRekeyIndependent()
		ids[i] = LaneID(fmt.Sprintf("test-wide-lane-%03d", i))
		specs[i].id = ids[i]
	}
	catalog := newLaneCatalog(specs)
	selected := NewLaneSet(ids[129], ids[64], ids[0], ids[63])
	domain, err := catalog.TryProductDomainWithLaneSet(reg, selected)
	if err != nil {
		t.Fatal(err)
	}
	inventory := domain.LaneInventory()
	wantIDs := []LaneID{ids[0], ids[63], ids[64], ids[129]}
	if len(inventory) != len(wantIDs) {
		t.Fatalf("wide inventory length = %d, want %d", len(inventory), len(wantIDs))
	}
	for i := range inventory {
		if inventory[i].ID() != wantIDs[i] || inventory[i].Ordinal() != LaneOrdinal(i) {
			t.Fatalf("wide inventory[%d] = (%q,%d), want (%q,%d)", i, inventory[i].ID(), inventory[i].Ordinal(), wantIDs[i], i)
		}
	}
	mask := domain.Lattice().Bottom().laneMask
	for _, ordinal := range []laneBit{0, 63, 64, 129} {
		if !mask.allows(ordinal) {
			t.Errorf("selected wide lane %d is disabled", ordinal)
		}
	}
	for _, ordinal := range []laneBit{1, 62, 65, 128} {
		if mask.allows(ordinal) {
			t.Errorf("unselected wide lane %d is enabled", ordinal)
		}
	}
	factors, err := domain.Decompose(domain.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.Compose(factors); err != nil {
		t.Fatalf("wide factor round trip: %v", err)
	}
}

func TestProductLaneInventoryUsesCatalogOrderAndOrdinals(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneNumCeils, LaneValues})
	if err != nil {
		t.Fatal(err)
	}
	inventory := domain.LaneInventory()
	if len(inventory) != 2 {
		t.Fatalf("lane inventory length = %d, want 2", len(inventory))
	}
	want := []LaneID{LaneValues, LaneNumCeils}
	for i, lane := range inventory {
		if lane.ID() != want[i] || lane.Ordinal() != LaneOrdinal(i) {
			t.Fatalf("lane[%d] = (%q,%d), want (%q,%d)", i, lane.ID(), lane.Ordinal(), want[i], i)
		}
		lookedUp, ok := domain.ProductLane(lane.ID())
		if !ok || lookedUp != lane {
			t.Fatalf("ProductLane(%q) = (%v,%v), want exact inventory descriptor", lane.ID(), lookedUp, ok)
		}
	}
	if _, ok := domain.ProductLane(LanePathEvidence); ok {
		t.Fatal("disabled lane is present in product inventory")
	}

	// Inventory slices are caller-owned.
	inventory[0] = ProductLane{}
	if got := domain.LaneInventory()[0].ID(); got != LaneValues {
		t.Fatalf("caller mutation changed product inventory to %q", got)
	}
}

func TestProductLaneDecomposeComposeRoundTripEveryRegisteredLane(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	lattice := domain.Lattice()
	value := Reachable(State{})
	for _, sample := range stateLawLaneSamples(reg, keys) {
		value = lattice.Join(value, sample.state)
	}
	value = NormalizeForDomain(lattice, value)

	factors, err := domain.Decompose(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(factors) != len(domain.LaneInventory()) {
		t.Fatalf("factor count = %d, inventory = %d", len(factors), len(domain.LaneInventory()))
	}
	if len(factors) != 17 {
		t.Fatalf("registered factor count = %d, want all 17 State lanes", len(factors))
	}
	for i, factor := range factors {
		if factor.Lane() != domain.LaneInventory()[i] {
			t.Fatalf("factor %d lane = %v, want %v", i, factor.Lane(), domain.LaneInventory()[i])
		}
		if _, err := domain.LaneSame(factor, factor); err != nil {
			t.Fatalf("LaneSame for lane %q: %v", factor.Lane().ID(), err)
		}
	}
	recomposed, err := domain.Compose(factors)
	if err != nil {
		t.Fatal(err)
	}
	if !lattice.Equal(recomposed, value) {
		t.Fatal("Compose(Decompose(value)) changed State semantics")
	}
}

func TestProductLaneSameUsesRegisteredPersistentIdentity(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues})
	if err != nil {
		t.Fatal(err)
	}
	value := presentValue(reg)
	sharedState := State{}.WriteValue(reg, statekey.SymbolValue(1), value)
	shared := mustOnlyLaneFactor(t, domain, sharedState)
	if same, err := domain.LaneSame(shared, shared); err != nil || !same {
		t.Fatalf("LaneSame(shared, shared) = (%v,%v), want true", same, err)
	}

	// Independent writes produce distinct persistent maps with equal contents.
	// Same is an O(1) positive identity check, not a replacement for Equal.
	distinctLeft := mustOnlyLaneFactor(t, domain, State{}.WriteValue(reg, statekey.SymbolValue(2), value))
	distinctRight := mustOnlyLaneFactor(t, domain, State{}.WriteValue(reg, statekey.SymbolValue(2), value))
	if equal, err := domain.LaneEqual(distinctLeft, distinctRight); err != nil || !equal {
		t.Fatalf("LaneEqual(distinct equal factors) = (%v,%v), want true", equal, err)
	}
	if same, err := domain.LaneSame(distinctLeft, distinctRight); err != nil || same {
		t.Fatalf("LaneSame(distinct equal factors) = (%v,%v), want false", same, err)
	}
}

func TestProductLaneSameRejectsMismatchedAndForeignFactors(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues, LaneNumFloors})
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.Decompose(domain.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.LaneSame(factors[0], factors[1]); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("LaneSame(mismatched lanes) error = %v, want ErrInvalidLaneFactor", err)
	}

	foreignReg := standard.Registry()
	foreign := RegisteredProductDomain(foreignReg)
	foreignFactors, err := foreign.Decompose(foreign.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.LaneSame(factors[0], foreignFactors[0]); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("LaneSame(foreign factor) error = %v, want ErrInvalidLaneFactor", err)
	}
}

func TestProductLaneOperationsAgreeWithSingleLaneDomains(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	for _, sample := range stateLawLaneSamples(reg, keys) {
		t.Run(string(sample.lane), func(t *testing.T) {
			domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{sample.lane})
			if err != nil {
				t.Fatal(err)
			}
			lattice := domain.Lattice()
			lane := domain.LaneInventory()[0]
			bottomState := lattice.Bottom()
			sampleState := NormalizeForDomain(lattice, sample.state)
			topState := lattice.Top()
			bottom := mustOnlyLaneFactor(t, domain, bottomState)
			representative := mustOnlyLaneFactor(t, domain, sampleState)
			top := mustOnlyLaneFactor(t, domain, topState)

			gotBottom, err := domain.LaneBottom(lane)
			if err != nil {
				t.Fatal(err)
			}
			gotTop, err := domain.LaneTop(lane)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorEqual(t, domain, gotBottom, bottom, "bottom")
			assertLaneFactorEqual(t, domain, gotTop, top, "top")

			if got, err := domain.LaneEqual(representative, representative); err != nil || !got {
				t.Fatalf("LaneEqual(self) = (%v,%v), want true", got, err)
			}
			if got, err := domain.LaneLessOrEq(bottom, representative); err != nil || got != lattice.LessOrEq(bottomState, sampleState) {
				t.Fatalf("LaneLessOrEq = (%v,%v), whole State = %v", got, err, lattice.LessOrEq(bottomState, sampleState))
			}

			joined, err := domain.LaneJoin(bottom, representative)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorComposesTo(t, domain, joined, lattice.Join(bottomState, sampleState), "join")
			widened, err := domain.LaneWiden(bottom, representative)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorComposesTo(t, domain, widened, lattice.Widen(bottomState, sampleState), "widen")
			narrowed, err := domain.LaneNarrow(top, representative)
			if err != nil {
				t.Fatal(err)
			}
			assertLaneFactorComposesTo(t, domain, narrowed, lattice.Narrow(topState, sampleState), "narrow")

			fingerprintConfig := FingerprintConfig{Registry: reg, KeySpace: keys, Lanes: []LaneID{sample.lane}}
			first, err := domain.LaneFingerprint(fingerprintConfig, representative)
			if err != nil {
				t.Fatal(err)
			}
			second, err := domain.LaneFingerprint(fingerprintConfig, representative)
			if err != nil || first != second {
				t.Fatalf("lane fingerprint = (%d,%v), want deterministic %d", second, err, first)
			}
		})
	}
}

func TestProductLaneComposeFailsClosedOnIncompleteOrForeignFactors(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues, LaneNumFloors})
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.Decompose(domain.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.Compose(factors[:1]); !errors.Is(err, ErrIncompleteLaneFactors) {
		t.Fatalf("Compose(incomplete) error = %v, want ErrIncompleteLaneFactors", err)
	}
	reversed := []LaneFactor{factors[1], factors[0]}
	if _, err := domain.Compose(reversed); !errors.Is(err, ErrIncompleteLaneFactors) {
		t.Fatalf("Compose(reordered) error = %v, want ErrIncompleteLaneFactors", err)
	}
	foreign := DefaultLaneCatalog().ProductDomain(reg)
	foreignFactors, err := foreign.Decompose(foreign.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	factors[0] = foreignFactors[0]
	if _, err := domain.Compose(factors); !errors.Is(err, ErrIncompleteLaneFactors) {
		t.Fatalf("Compose(foreign) error = %v, want ErrIncompleteLaneFactors", err)
	}
}

func TestProductLaneValueDependenciesAreVisitedPerRegisteredLane(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	fx := stateLawFixtureFor(reg, keys)
	value := State{}.WritePathKey(reg, keys, fx.pathKey, fx.present).AddBranchProof(fx.proof)
	factors, err := domain.Decompose(value)
	if err != nil {
		t.Fatal(err)
	}
	want := map[statekey.Value]struct{}{fx.valueSlot: {}}
	for _, factor := range factors {
		got := make(map[statekey.Value]struct{})
		if err := domain.VisitLaneValueDependencies(factor, keys, func(dependency statekey.ValueDependency) {
			slot, concrete := dependency.Concrete()
			if concrete {
				got[slot] = struct{}{}
			}
		}); err != nil {
			t.Fatal(err)
		}
		if factor.Lane().ID() == LanePathEvidence {
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("path-evidence dependencies = %v, want %v", got, want)
			}
		} else if len(got) != 0 {
			t.Fatalf("Values-independent lane %q reported dependencies %v", factor.Lane().ID(), got)
		}
	}
}

func TestProductLaneFactorizationIsCatalogOwnedNotLaneIDSwitched(t *testing.T) {
	reg := standard.Registry()
	customID := LaneID("test-custom-values")
	customSpec := valuesLaneSpec
	customSpec.id = customID
	catalog := newLaneCatalog([]laneSpec{customSpec})
	domain := catalog.ProductDomain(reg)
	inventory := domain.LaneInventory()
	if len(inventory) != 1 || inventory[0].ID() != customID {
		t.Fatalf("custom inventory = %v, want [%q]", inventory, customID)
	}
	value := State{}.WriteValue(reg, statekey.Value(1), presentValue(reg))
	factors, err := domain.Decompose(value)
	if err != nil {
		t.Fatal(err)
	}
	recomposed, err := domain.Compose(factors)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Lattice().Equal(recomposed, NormalizeForDomain(domain.Lattice(), value)) {
		t.Fatal("custom registered lane did not round-trip through factor API")
	}
}

func TestProductSparseFactorAPIsUseOnlySealedRequestedLanes(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	lattice := domain.Lattice()
	value := lattice.Bottom()
	for _, sample := range stateLawLaneSamples(reg, keys) {
		value = lattice.Join(value, sample.state)
	}

	inventory := domain.NonValuesLaneInventory()
	if len(inventory) != len(domain.LaneInventory())-1 {
		t.Fatalf("non-Values inventory size = %d", len(inventory))
	}
	for _, lane := range inventory {
		if lane.ID() == LaneValues {
			t.Fatal("non-Values inventory contains Values")
		}
	}
	requested := []ProductLane{inventory[len(inventory)-1], inventory[0]}
	factors, err := domain.DecomposeLanes(value, requested)
	if err != nil {
		t.Fatal(err)
	}
	if len(factors) != len(requested) || factors[0].Lane() != requested[0] || factors[1].Lane() != requested[1] {
		t.Fatal("selective decomposition did not preserve caller order")
	}
	got, err := domain.ComposeSparse(factors)
	if err != nil {
		t.Fatal(err)
	}
	writes := NewLaneSet(requested[0].ID(), requested[1].ID())
	want, err := domain.PatchFactors(lattice.Bottom(), value, writes)
	if err != nil {
		t.Fatal(err)
	}
	if !lattice.Equal(got, want) {
		t.Fatal("sparse composition differs from exact selected-lane patch")
	}

	if _, err := domain.DecomposeLanes(value, []ProductLane{requested[0], requested[0]}); !errors.Is(err, ErrInvalidProductLane) {
		t.Fatalf("duplicate descriptor error = %v", err)
	}
	if _, err := domain.ComposeSparse([]LaneFactor{factors[0], factors[0]}); !errors.Is(err, ErrIncompleteLaneFactors) {
		t.Fatalf("duplicate factor error = %v", err)
	}
	foreignDomain := DefaultLaneCatalog().ProductDomain(reg)
	if _, err := domain.DecomposeLanes(value, foreignDomain.NonValuesLaneInventory()[:1]); !errors.Is(err, ErrInvalidProductLane) {
		t.Fatalf("foreign descriptor error = %v", err)
	}
}

func mustOnlyLaneFactor(t *testing.T, domain ProductDomain, value State) LaneFactor {
	t.Helper()
	factors, err := domain.Decompose(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(factors) != 1 {
		t.Fatalf("factor count = %d, want 1", len(factors))
	}
	return factors[0]
}

func assertLaneFactorEqual(t *testing.T, domain ProductDomain, got, want LaneFactor, operation string) {
	t.Helper()
	equal, err := domain.LaneEqual(got, want)
	if err != nil || !equal {
		t.Fatalf("%s factor equality = (%v,%v), want true", operation, equal, err)
	}
}

func assertLaneFactorComposesTo(t *testing.T, domain ProductDomain, factor LaneFactor, want State, operation string) {
	t.Helper()
	got, err := domain.Compose([]LaneFactor{factor})
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Lattice().Equal(got, want) {
		t.Fatalf("%s factor differs from whole-State operation", operation)
	}
}
