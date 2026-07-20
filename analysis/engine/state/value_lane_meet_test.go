package state

import (
	"testing"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestValueLaneExactLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	domain := valueLaneDomain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)
	symbol := statekey.SymbolValue(1)
	result := statekey.ReturnSlot(0)
	latticelaws.LawSuite[valueLane]{
		Name:   "state.valueLane",
		Domain: domain,
		Sample: []valueLane{
			domain.Bottom(),
			domain.Top(),
			{symbols: map[statekey.Value]product.Value{symbol: present}},
			{returns: map[statekey.Value]product.Value{result: absent}},
			{
				symbols: map[statekey.Value]product.Value{symbol: product.Top()},
				returns: map[statekey.Value]product.Value{result: present},
			},
		},
		WideningBound: 8,
	}.Run(t)
}

func TestGenericValueFactorExactLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	domain := ValueFactorLattice[string](reg)
	present := presentValue(reg)
	absent := absentValue(reg)
	latticelaws.LawSuite[ValueFactor[string]]{
		Name:   "state.ValueFactor[string]",
		Domain: domain,
		Sample: []ValueFactor[string]{
			domain.Bottom(),
			domain.Top(),
			{Values: map[string]product.Value{"in": present}},
			{Values: map[string]product.Value{"out": absent}},
			{Values: map[string]product.Value{"in": product.Top(), "out": present}},
		},
		WideningBound: 8,
	}.Run(t)
}

func TestValuesLaneRegistersExactPointwiseMeet(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LaneValues)
	if !ok {
		t.Fatal("values lane is not registered")
	}

	symbol := statekey.SymbolValue(1)
	leftOnly := statekey.SymbolValue(2)
	rightOnly := statekey.SymbolValue(3)
	result := statekey.ReturnSlot(0)
	present := presentValue(reg)
	absent := absentValue(reg)

	leftState := State{}.
		WriteValue(reg, symbol, present).
		WriteValue(reg, leftOnly, present).
		WriteValue(reg, result, product.Top())
	rightState := State{}.
		WriteValue(reg, symbol, product.Top()).
		WriteValue(reg, rightOnly, absent).
		WriteValue(reg, result, absent)
	left := mustOnlyLaneFactor(t, domain, leftState)
	right := mustOnlyLaneFactor(t, domain, rightState)

	met, err := domain.LaneMeet(left, right)
	if err != nil {
		t.Fatalf("registered Meet: %v", err)
	}
	got, err := domain.Compose([]LaneFactor{met})
	if err != nil {
		t.Fatal(err)
	}
	if !product.Equal(reg, got.ReadValue(reg, symbol), present) {
		t.Fatal("symbol Meet did not preserve the exact present constraint")
	}
	if !product.Equal(reg, got.ReadValue(reg, result), absent) {
		t.Fatal("return Meet did not preserve the exact absent constraint")
	}
	bottom := product.Bottom(reg)
	for _, slot := range []statekey.Value{leftOnly, rightOnly} {
		if !product.Equal(reg, got.ReadValue(reg, slot), bottom) {
			t.Fatalf("one-sided slot %d survived pointwise Meet", slot)
		}
	}

	joined, err := domain.LaneJoin(left, right)
	if err != nil {
		t.Fatal(err)
	}
	absorbed, err := domain.LaneMeet(left, joined)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, absorbed, left, "meet absorption")

	top, err := domain.LaneTop(lane)
	if err != nil {
		t.Fatal(err)
	}
	topIdentity, err := domain.LaneMeet(top, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, topIdentity, left, "top identity")

	bottomFactor, err := domain.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	bottomAbsorbed, err := domain.LaneMeet(bottomFactor, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, bottomAbsorbed, bottomFactor, "bottom absorption")
}

func TestValueLaneMeetReusesTopIdentityOperand(t *testing.T) {
	reg := standard.Registry()
	domain := valueLaneDomain(reg)
	values := map[statekey.Value]product.Value{statekey.SymbolValue(1): presentValue(reg)}
	finite := valueLane{symbols: values}

	if got := domain.Meet(domain.Top(), finite); !domain.Same(got, finite) {
		t.Fatal("Meet(Top, finite) did not reuse the finite persistent maps")
	}
	if got := domain.Meet(finite, domain.Top()); !domain.Same(got, finite) {
		t.Fatal("Meet(finite, Top) did not reuse the finite persistent maps")
	}
}
