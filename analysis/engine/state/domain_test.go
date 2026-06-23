package state

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
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

	valueOnly := DomainWithLaneSet(reg, NewLaneSet(LaneValues))
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

	valueAndFrozen := DomainWithLaneSet(reg, NewLaneSet(LaneValues).With(LaneFrozenTables))
	if valueAndFrozen.Equal(valueState, both) {
		t.Fatal("value+frozen domain ignored enabled frozen-table lane")
	}
	joinedBoth := valueAndFrozen.Join(both, frozenState)
	if !product.Domain(reg).Equal(joinedBoth.ReadValue(reg, slot), value) || !joinedBoth.IsTableFrozen(tableID) {
		t.Fatalf("value+frozen join = %s frozen=%v, want value and frozen", formatValue(reg, joinedBoth.ReadValue(reg, slot)), joinedBoth.IsTableFrozen(tableID))
	}
}

func TestNormalizeForDomainDropsDisabledLanes(t *testing.T) {
	reg := standard.Registry()
	slot := key.SymbolValue(symbol.ID(13))
	value := presentValue(reg)
	tableID := identity.ID{Kind: "table", Site: "domain-normalize-lanes", Index: 1}

	valueOnly := DomainWithLanes(reg, []LaneID{LaneValues})
	got := NormalizeForDomain(valueOnly, State{}.WriteValue(reg, slot, value).FreezeTable(tableID))
	if read := got.ReadValue(reg, slot); !product.Domain(reg).Equal(read, value) {
		t.Fatalf("NormalizeForDomain value = %s, want %s", formatValue(reg, read), formatValue(reg, value))
	}
	if got.IsTableFrozen(tableID) {
		t.Fatal("NormalizeForDomain preserved disabled frozen-table lane")
	}

	empty := DomainWithLanes(reg, []LaneID{})
	got = NormalizeForDomain(empty, State{}.WriteValue(reg, slot, value).FreezeTable(tableID))
	if read := got.ReadValue(reg, slot); !product.Domain(reg).Equal(read, product.Bottom(reg)) {
		t.Fatalf("NormalizeForDomain empty-domain value = %s, want bottom", formatValue(reg, read))
	}
	if got.IsTableFrozen(tableID) {
		t.Fatal("NormalizeForDomain empty-domain preserved frozen-table lane")
	}
}

func TestDomainWithLanesCopiesCallerSlice(t *testing.T) {
	reg := standard.Registry()
	slot := key.SymbolValue(symbol.ID(11))
	value := presentValue(reg)
	tableID := identity.ID{Kind: "table", Site: "domain-lanes-copy", Index: 1}

	lanes := []LaneID{LaneValues}
	valueOnly := DomainWithLanes(reg, lanes)
	lanes[0] = LaneFrozenTables

	valueState := State{}.WriteValue(reg, slot, value)
	frozenState := State{}.FreezeTable(tableID)
	both := valueState.FreezeTable(tableID)

	if !valueOnly.Equal(valueState, both) {
		t.Fatal("DomainWithLanes kept caller storage and changed enabled lanes after construction")
	}
	joined := valueOnly.Join(valueState, frozenState)
	if got := joined.ReadValue(reg, slot); !product.Domain(reg).Equal(got, value) {
		t.Fatalf("DomainWithLanes copied domain lost enabled value lane: got %s want %s", formatValue(reg, got), formatValue(reg, value))
	}
	if joined.IsTableFrozen(tableID) {
		t.Fatal("DomainWithLanes copied domain preserved lane added by caller slice mutation")
	}
}

func TestDefaultLanesExposeEveryStateAxis(t *testing.T) {
	reg := standard.Registry()
	expected := []LaneID{
		LaneValues,
		LanePathEvidence,
		LaneDynamicIndex,
		LaneHeapTableIdentity,
		LaneFrozenTables,
		LaneEffectDeltas,
		LaneEscapeEvents,
		LaneChannelSelect,
		LaneStoreRelations,
		LaneTypestates,
		LanePlacement,
		LaneLenFloors,
		LaneNumFloors,
		LaneDiffRelations,
	}

	if got := DefaultLanes(); !reflect.DeepEqual(got, expected) {
		t.Fatalf("DefaultLanes() = %#v, want every exported state axis in registry order %#v", got, expected)
	}
	if got := DefaultLaneCatalog().LaneSet().IDs(); !reflect.DeepEqual(got, expected) {
		t.Fatalf("DefaultLaneCatalog().LaneSet() = %#v, want %#v", got, expected)
	}

	if _, err := TryDomainWithLanes(reg, expected); err != nil {
		t.Fatalf("TryDomainWithLanes(all axes) error = %v", err)
	}
	for _, lane := range expected {
		t.Run(string(lane), func(t *testing.T) {
			single, err := TryDomainWithLanes(reg, []LaneID{lane})
			if err != nil {
				t.Fatalf("TryDomainWithLanes(single %q) error = %v", lane, err)
			}
			if !single.Equal(single.Bottom(), single.Bottom()) {
				t.Fatalf("single-lane domain %q does not recognize its own bottom", lane)
			}
			if !single.Equal(single.Top(), single.Top()) {
				t.Fatalf("single-lane domain %q does not recognize its own top", lane)
			}

			without := NewLaneSet(expected...).Without(lane)
			if without.Has(lane) {
				t.Fatalf("Without(%q) kept disabled lane in %#v", lane, without.IDs())
			}
			if _, err := TryDomainWithLaneSet(reg, without); err != nil {
				t.Fatalf("TryDomainWithLaneSet(without %q) error = %v", lane, err)
			}
		})
	}
}

func TestReachableTransitionCoversRegisteredMustFactLanes(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	slot := key.SymbolValue(symbol.ID(12))

	state := Domain(reg).Bottom().WriteValue(reg, slot, presentValue(reg))
	if state.PathRefinementsSnapshot(ks).Bottom {
		t.Fatal("reachable state kept path-evidence lane at bottom")
	}
	if state.FrozenTablesSnapshot().Bottom {
		t.Fatal("reachable state kept frozen-table lane at bottom")
	}
	if state.EscapeEventsSnapshot().Bottom {
		t.Fatal("reachable state kept escape-event lane at bottom")
	}
	if state.ChannelSelectFactsSnapshot().Bottom {
		t.Fatal("reachable state kept channel-select lane at bottom")
	}
	if state.StoreRelationsSnapshot().Bottom {
		t.Fatal("reachable state kept store-relation lane at bottom")
	}
	if state.lenFloors.lane.Bottom() {
		t.Fatal("reachable state kept length-floor lane at bottom")
	}
	if state.NumFloorsSnapshot(ks).Bottom {
		t.Fatal("reachable state kept numeric-floor lane at bottom")
	}
	if state.RelConstraints().Bottom {
		t.Fatal("reachable state kept diff-relation lane at bottom")
	}
}

func TestLaneSetValidatesAndCopiesSelection(t *testing.T) {
	reg := standard.Registry()

	catalog := DefaultLaneCatalog()
	lanes := catalog.LaneSet()
	if lanes.Len() == 0 || lanes.At(0) != LaneValues {
		t.Fatalf("default lanes = %#v, want values first", lanes.IDs())
	}
	if err := catalog.ValidateLaneSet(lanes); err != nil {
		t.Fatalf("default lane catalog is invalid: %v", err)
	}
	mutatedIDs := lanes.IDs()
	mutatedIDs[0] = LaneID("mutated")
	if got := catalog.LaneSet().At(0); got != LaneValues {
		t.Fatalf("LaneCatalog.LaneSet returned shared storage; first lane = %s", got)
	}
	if got := DefaultLaneSet().At(0); got != LaneValues {
		t.Fatalf("DefaultLaneSet returned shared storage; first lane = %s", got)
	}
	ids := DefaultLaneSet().IDs()
	ids[0] = LaneID("mutated")
	if got := DefaultLanes()[0]; got != LaneValues {
		t.Fatalf("DefaultLanes returned shared storage; first lane = %s", got)
	}
	source := []LaneID{LaneValues}
	copied := NewLaneSet(source...)
	source[0] = LaneID("mutated")
	if got := copied.At(0); got != LaneValues {
		t.Fatalf("NewLaneSet kept caller storage; first lane = %s", got)
	}

	withoutFrozen := DefaultLaneSet().Without(LaneFrozenTables)
	if withoutFrozen.Has(LaneFrozenTables) {
		t.Fatal("Without kept disabled frozen-table lane")
	}
	withFrozen := withoutFrozen.With(LaneFrozenTables)
	if !withFrozen.Has(LaneFrozenTables) {
		t.Fatal("With did not add frozen-table lane")
	}

	if err := catalog.ValidateLaneSet(NewLaneSet(LaneValues, LaneFrozenTables)); err != nil {
		t.Fatalf("ValidateLaneSet(valid) error = %v", err)
	}
	if _, err := catalog.TryDomainWithLaneSet(reg, NewLaneSet(LaneValues)); err != nil {
		t.Fatalf("TryDomainWithLaneSet(valid) error = %v", err)
	}
	if _, err := TryDomainWithLanes(reg, []LaneID{LaneValues}); err != nil {
		t.Fatalf("TryDomainWithLanes(valid) error = %v", err)
	}
	if err := catalog.ValidateLaneSet(NewLaneSet(LaneID("not-a-lane"))); err == nil || !strings.Contains(err.Error(), `unknown lane "not-a-lane"`) {
		t.Fatalf("ValidateLaneSet(unknown) error = %v, want unknown lane", err)
	}
	if _, err := catalog.TryDomainWithLaneSet(reg, NewLaneSet(LaneID("not-a-lane"))); err == nil || !strings.Contains(err.Error(), `unknown lane "not-a-lane"`) {
		t.Fatalf("TryDomainWithLaneSet(unknown) error = %v, want unknown lane", err)
	}
	if _, err := TryDomainWithLanes(reg, []LaneID{LaneValues, LaneValues}); err == nil || !strings.Contains(err.Error(), `duplicate lane "values"`) {
		t.Fatalf("TryDomainWithLanes(duplicate) error = %v, want duplicate lane", err)
	}

	requirePanic(t, func() {
		_ = catalog.DomainWithLaneSet(reg, NewLaneSet(LaneID("not-a-lane")))
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
