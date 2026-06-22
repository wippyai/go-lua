package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDomainStableAcrossRepeatedConstruction(t *testing.T) {
	reg := standard.Registry()
	top := Domain(reg).Top()
	bottom := Domain(reg).Bottom()
	domain := Domain(reg)

	if !domain.Equal(top, domain.Top()) {
		t.Fatalf("reconstructed state domain did not recognize prior top")
	}
	if !domain.Equal(bottom, domain.Bottom()) {
		t.Fatalf("reconstructed state domain did not recognize prior bottom")
	}
	if !domain.Equal(domain.Join(bottom, top), top) {
		t.Fatalf("reconstructed state domain join(bottom, top) did not produce top")
	}
}

func TestDomainWithLaneSetSelectsIndependentAxes(t *testing.T) {
	reg := standard.Registry()
	slot := key.SymbolValue(symbol.ID(10))
	value := presentValue(reg)
	tableID := identity.ID{Kind: "table", Site: "domain-lanes", Index: 1}

	valueState := State{}.WriteValue(reg, slot, value)
	frozenState := State{}.FreezeTable(tableID)
	both := valueState.FreezeTable(tableID)

	valueOnly := DomainWithLaneSet(reg, LaneSet{LaneValues})
	if !valueOnly.Equal(valueState, both) {
		t.Fatal("value-only domain considered disabled frozen-table lane")
	}
	joinedValueOnly := valueOnly.Join(valueState, frozenState)
	if got := joinedValueOnly.ReadValue(reg, slot); !product.Domain(reg).Equal(got, value) {
		t.Fatalf("value-only join value = %s, want %s", formatValue(reg, got), formatValue(reg, value))
	}
	if joinedValueOnly.IsTableFrozen(tableID) {
		t.Fatal("value-only join preserved disabled frozen-table lane")
	}

	valueAndFrozen := DomainWithLaneSet(reg, LaneSet{LaneValues}.With(LaneFrozenTables))
	if valueAndFrozen.Equal(valueState, both) {
		t.Fatal("value+frozen domain ignored enabled frozen-table lane")
	}
	joinedBoth := valueAndFrozen.Join(both, frozenState)
	if !product.Domain(reg).Equal(joinedBoth.ReadValue(reg, slot), value) || !joinedBoth.IsTableFrozen(tableID) {
		t.Fatalf("value+frozen join = %s frozen=%v, want value and frozen", formatValue(reg, joinedBoth.ReadValue(reg, slot)), joinedBoth.IsTableFrozen(tableID))
	}
}

func TestDomainLaneSetValidatesAndCopiesSelection(t *testing.T) {
	reg := standard.Registry()

	catalog := DefaultLaneCatalog()
	lanes := catalog.LaneSet()
	if len(lanes) == 0 || lanes[0] != LaneValues {
		t.Fatalf("default lanes = %#v, want values first", lanes)
	}
	lanes[0] = LaneID("mutated")
	if got := catalog.LaneSet()[0]; got != LaneValues {
		t.Fatalf("LaneCatalog.LaneSet returned shared storage; first lane = %s", got)
	}
	if got := DefaultDomainLaneSet()[0]; got != LaneValues {
		t.Fatalf("DefaultDomainLaneSet returned shared storage; first lane = %s", got)
	}
	ids := DefaultDomainLaneSet().IDs()
	ids[0] = LaneID("mutated")
	if got := DefaultDomainLanes()[0]; got != LaneValues {
		t.Fatalf("DefaultDomainLanes returned shared storage; first lane = %s", got)
	}

	withoutFrozen := DefaultDomainLaneSet().Without(LaneFrozenTables)
	if withoutFrozen.Has(LaneFrozenTables) {
		t.Fatal("Without kept disabled frozen-table lane")
	}
	withFrozen := withoutFrozen.With(LaneFrozenTables)
	if !withFrozen.Has(LaneFrozenTables) {
		t.Fatal("With did not add frozen-table lane")
	}

	requirePanic(t, func() {
		_ = catalog.DomainWithLaneSet(reg, LaneSet{LaneID("not-a-lane")})
	})
	requirePanic(t, func() {
		_ = DomainWithLanes(reg, []LaneID{LaneValues, LaneValues})
	})
}

func TestLaneCatalogDomainMatchesPackageDomain(t *testing.T) {
	reg := standard.Registry()
	catalogDomain := DefaultLaneCatalog().Domain(reg)
	packageDomain := Domain(reg)

	if !packageDomain.Equal(catalogDomain.Bottom(), packageDomain.Bottom()) {
		t.Fatal("default lane catalog bottom differs from package Domain bottom")
	}
	if !packageDomain.Equal(catalogDomain.Top(), packageDomain.Top()) {
		t.Fatal("default lane catalog top differs from package Domain top")
	}
}
