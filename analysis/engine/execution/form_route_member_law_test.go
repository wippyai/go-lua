package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// routedPlane seals one narrowed plane over the fixture's own paired geometry,
// for a rule that declares a routed publication.
func routedPlane(t *testing.T, fixture selectedFixture) FormPlane[uint64, uint64] {
	t.Helper()
	table, tableOK := NewRouteTable(fixture.units, fixture.targets)
	if !tableOK {
		t.Fatal("route geometry")
	}
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, table, make([]ForeignFactor, 3), nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	narrowed, narrowedOK := plane.forRule([]FormRow{{Member: 0, Form: FormSelectedRoute, Rule: routedDescriptor(t, 1)}})
	if !narrowedOK {
		t.Fatal("narrowed plane")
	}
	return narrowed
}

// TestARouteMemberIsTheWholeRouteOrNothing states what the pair is for. A
// member of a derived relation is a coordinate to observe AND a destination to
// publish at; resolving one without the other would let a row read one member
// and write at another's destination, which is exactly the corruption a
// re-derived relation produces. So one dense position of one authenticated
// geometry answers both halves, or answers nothing.
func TestARouteMemberIsTheWholeRouteOrNothing(t *testing.T) {
	fixture := newSelectedFixture(t)
	plane := routedPlane(t, fixture)
	for index := 0; index < selectedFixtureWidth; index++ {
		member, resolved := plane.RouteMember(uint32(index), uint32(index), uint64(index)+1)
		if !resolved || !member.Valid() {
			t.Fatalf("dense %d resolved no route member", index)
		}
		if !member.Coordinate().Unit.Same(fixture.units[index]) {
			t.Fatalf("dense %d reads at a coordinate the Factor did not pair with it", index)
		}
		if member.target != fixture.targets[index] {
			t.Fatalf("dense %d publishes at a target the Factor did not pair with it", index)
		}
		if member.Tag() != uint64(index)+1 {
			t.Fatalf("dense %d carries tag %d, want the one its relation issued", index, member.Tag())
		}
	}
	if _, resolved := plane.RouteMember(selectedFixtureWidth, selectedFixtureWidth, 1); resolved {
		t.Fatal("a dense position outside the geometry resolved a route")
	}
}

// TestRouteGeometryIsOnlyVisibleToARuleThatDeclaredARoute is the fence half.
// Route geometry is Factor-owned data every rule of that Factor could
// otherwise address; a rule whose plan states no routed publication has no
// business resolving a destination, in the same way it has no business reading
// a Factor its joins never named. The narrowing states both at once.
func TestRouteGeometryIsOnlyVisibleToARuleThatDeclaredARoute(t *testing.T) {
	fixture := newSelectedFixture(t)
	table, tableOK := NewRouteTable(fixture.units, fixture.targets)
	if !tableOK {
		t.Fatal("route geometry")
	}
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, table, make([]ForeignFactor, 3), nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	if _, resolved := plane.RouteMember(0, 0, 1); resolved {
		t.Fatal("an unnarrowed plane resolved a route member")
	}
	if width := plane.RouteWidth(); width != 0 {
		t.Fatalf("an unnarrowed plane published route width %d", width)
	}
	exact, exactOK := plane.forRule([]FormRow{{Member: 0, Form: FormExact, Rule: planCompiledExactRule(t)}})
	if !exactOK {
		t.Fatal("narrowed exact plane")
	}
	if _, resolved := exact.RouteMember(0, 0, 1); resolved {
		t.Fatal("a rule that declares no route resolved a route member")
	}
	if width := exact.RouteWidth(); width != 0 {
		t.Fatalf("a rule that declares no route sees route width %d", width)
	}
	if width := routedPlane(t, fixture).RouteWidth(); width != selectedFixtureWidth {
		t.Fatalf("routed rule sees width %d, want the Factor's %d", width, selectedFixtureWidth)
	}
}

// TestARouteMemberIsAuthenticatedAgainstItsOwnFactor states that the geometry
// is not a byte range two Factors can both claim. A Unit and a Target another
// binding minted are a route in that binding and nothing here, so a plane
// handed a foreign geometry resolves no member rather than resolving one that
// would publish into another Factor's column.
func TestARouteMemberIsAuthenticatedAgainstItsOwnFactor(t *testing.T) {
	own := newSelectedFixture(t)
	foreign := newSelectedFixture(t)
	table, tableOK := NewRouteTable(foreign.units, foreign.targets)
	if !tableOK {
		t.Fatal("foreign geometry")
	}
	plane, planeOK := NewFormPlane(own.binding, nil, nil, table, make([]ForeignFactor, 3), nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	narrowed, narrowedOK := plane.forRule([]FormRow{{Member: 0, Form: FormSelectedRoute, Rule: routedDescriptor(t, 1)}})
	if !narrowedOK {
		t.Fatal("narrowed plane")
	}
	for index := 0; index < selectedFixtureWidth; index++ {
		if _, resolved := narrowed.RouteMember(uint32(index), uint32(index), 1); resolved {
			t.Fatalf("dense %d of a foreign geometry resolved as this Factor's route", index)
		}
	}
}

// TestRouteGeometryKeepsReadAndDestinationExtentsIndependent states the
// separate-coordinate invariant at the seal. The destination half is
// optional, and its extent need not equal the read coordinate extent: the
// plane authenticates each dense index when a route member is resolved.
func TestRouteGeometryKeepsReadAndDestinationExtentsIndependent(t *testing.T) {
	fixture := newSelectedFixture(t)
	if table, sealed := NewRouteTable(fixture.units, fixture.targets[:2]); !sealed || !table.Routed() {
		t.Fatal("a geometry with an independently sized destination space was refused")
	}
	if table, sealed := NewRouteTable(fixture.units[:2], fixture.targets); !sealed || !table.Routed() {
		t.Fatal("a geometry with an independently sized read space was refused")
	}
	empty, sealed := NewRouteTable(nil, nil)
	if !sealed || empty.Width() != 0 {
		t.Fatal("a Factor with no route universe has an empty geometry, not an invalid one")
	}
}

// TestARouteMemberCarriesNoHandleOnTheGeometry is the mutation law, stated in
// both directions. A member handed to a family is a value: writing to it
// cannot reach the Factor's tables, so a later lookup answers what the Factor
// sealed. And the plane publishes no slice of the geometry at all, so the only
// way to name a route is to resolve one, which is where authentication lives.
func TestARouteMemberCarriesNoHandleOnTheGeometry(t *testing.T) {
	fixture := newSelectedFixture(t)
	plane := routedPlane(t, fixture)
	member, resolved := plane.RouteMember(0, 0, 1)
	if !resolved {
		t.Fatal("route member")
	}
	member.coordinate = SelectedCoordinate{Unit: fixture.units[1], Tag: 99}
	member.target = fixture.targets[1]
	again, resolvedAgain := plane.RouteMember(0, 0, 1)
	if !resolvedAgain {
		t.Fatal("route member after mutation")
	}
	if !again.Coordinate().Unit.Same(fixture.units[0]) || again.target != fixture.targets[0] || again.Tag() != 1 {
		t.Fatal("a mutated member reached the Factor's sealed geometry")
	}
}

// TestARoutedRowPublishesTheRelationItObserved is fence three of the ruling,
// made structural. The cells and the members are the two halves of ONE
// materialized relation, so a fold handed cells observed at one member set and
// destinations from another is not a fold with a bug in it - it is a row that
// derived its relation twice, and it is refused.
func TestARoutedRowPublishesTheRelationItObserved(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, writeOK := NewRouteWrite(fixture.binding, 0)
	if !writeOK {
		t.Fatal("route write")
	}
	cells, members, routes := routeCells(fixture, 3)

	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	shifted := []RouteMember{members[1], members[2], members[0]}
	if outcome := FoldSelectedRoute(ticket, write, &scratch, cells, shifted, routes, routeLawReducer{empty: structure.NoSelection, failAt: -1}); outcome != structure.Refuse {
		t.Fatalf("a row that published a second member set settled %v, want refuse", outcome)
	}

	narrow := NewRun(1, 1)
	narrowTicket := issueSelected(t, narrow, fixture, fixture.state)
	var narrowScratch RouteScratch[uint64, uint64]
	if outcome := FoldSelectedRoute(narrowTicket, write, &narrowScratch, cells, members[:2], routes, routeLawReducer{empty: structure.NoSelection, failAt: -1}); outcome != structure.Refuse {
		t.Fatalf("a row with fewer destinations than observations settled %v, want refuse", outcome)
	}

	whole := NewRun(1, 1)
	wholeTicket := issueSelected(t, whole, fixture, fixture.state)
	var wholeScratch RouteScratch[uint64, uint64]
	if outcome := FoldSelectedRoute(wholeTicket, write, &wholeScratch, cells, members, routes, routeLawReducer{empty: structure.NoSelection, failAt: -1}); outcome != structure.Concrete {
		t.Fatalf("the observed relation settled %v, want concrete", outcome)
	}
}

// TestARouteMemberRefusesAWeakOrUnobservableCoordinate states what a geometry
// row must be for a route to exist at it: an exact observation and a strong
// publication. A weak destination cannot carry a routed row's fact, and a
// coordinate with no exact unit cannot be observed at all.
func TestARouteMemberRefusesAWeakOrUnobservableCoordinate(t *testing.T) {
	fixture := newSelectedFixture(t)
	units := append([]carrier.Unit(nil), fixture.units...)
	targets := append([]carrier.Target(nil), fixture.targets...)
	units[1] = carrier.Unit{}
	targets[2] = carrier.Target{}
	table, tableOK := NewRouteTable(units, targets)
	if !tableOK {
		t.Fatal("route geometry")
	}
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, table, make([]ForeignFactor, 3), nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	narrowed, narrowedOK := plane.forRule([]FormRow{{Member: 0, Form: FormSelectedRoute, Rule: routedDescriptor(t, 1)}})
	if !narrowedOK {
		t.Fatal("narrowed plane")
	}
	if _, resolved := narrowed.RouteMember(1, 1, 1); resolved {
		t.Fatal("a coordinate with no exact observation resolved a route")
	}
	if _, resolved := narrowed.RouteMember(2, 2, 1); resolved {
		t.Fatal("a coordinate with no strong destination resolved a route")
	}
	if _, resolved := narrowed.RouteMember(0, 0, 1); !resolved {
		t.Fatal("a whole geometry row resolved no route")
	}
}

// selectedOnlyDescriptor builds one rule that reads a dependent join and
// publishes at its own exact destination. It is the ordinary shape of a prior
// selection: the members it observes are not the members it writes.
func selectedOnlyDescriptor(t testing.TB) generated.CompiledRule {
	t.Helper()
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 2,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads: []generated.ReadPlan{
			{
				Input: 0, Factor: 1, Axis: 0,
				Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
				Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
				Form:        ruleprogram.Exact,
				PointBound:  ruleprogram.PointBound,
				Contract:    ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
				RowCapacity: 4, CellCapacity: 4,
			},
			{
				Input: 1, Factor: 1, Axis: 0,
				Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
				Predicate: ruleplan.ProjectionAddr{Axis: 0, Member: 1}, PredicatePresent: true,
				Form:        ruleprogram.Selected,
				PointBound:  ruleprogram.PointBound,
				Contract:    ruleplan.ReadContract{Order: ruleprogram.OrderByTag, Sparse: ruleprogram.SparseDefault, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
				Denominator: ruleplan.DenominatorAddr{Ordinal: 0, Present: true},
				RowCapacity: 4, CellCapacity: 4,
			},
		},
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
	})
	if !ok {
		t.Fatal("selected-only descriptor")
	}
	return rule
}

// TestADependentJoinResolvesCoordinatesAndNoDestinations states the ordinary
// dependent join, which is most of them. A rule selects the members of a
// relation in order to READ them; whether it also publishes through one of
// those joins is a separate statement its output makes.
//
// So the coordinate universe and the destination universe are fenced apart. A
// rule that selects and publishes exactly resolves members it can observe and
// no destination at all, which is what makes it impossible for such a fold to
// stage a routed write against a member it merely read.
func TestADependentJoinResolvesCoordinatesAndNoDestinations(t *testing.T) {
	fixture := newSelectedFixture(t)
	table, tableOK := NewRouteTable(fixture.units, fixture.targets)
	if !tableOK {
		t.Fatal("selection geometry")
	}
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, table, make([]ForeignFactor, 3), nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	selected, selectedOK := plane.forRule([]FormRow{{Member: 0, Form: FormExact, Rule: selectedOnlyDescriptor(t)}})
	if !selectedOK {
		t.Fatal("narrowed selected plane")
	}
	member, resolved := selected.SelectedMember(0, 1)
	if !resolved || !member.Valid() {
		t.Fatal("a rule that declares a dependent join resolved no member")
	}
	if member.Routed() {
		t.Fatal("a rule that publishes no route resolved a destination")
	}
	if !member.Coordinate().Unit.Same(fixture.units[0]) || member.Tag() != 1 {
		t.Fatal("the member does not name the coordinate its position holds")
	}
	if _, resolved := selected.RouteMember(0, 0, 1); resolved {
		t.Fatal("a rule that publishes no route resolved a whole route")
	}
	if width := selected.SelectedWidth(); width != selectedFixtureWidth {
		t.Fatalf("selected width = %d, want the Factor's %d", width, selectedFixtureWidth)
	}

	exact, exactOK := plane.forRule([]FormRow{{Member: 0, Form: FormExact, Rule: planCompiledExactRule(t)}})
	if !exactOK {
		t.Fatal("narrowed exact plane")
	}
	if _, resolved := exact.SelectedMember(0, 1); resolved {
		t.Fatal("a rule that declares no dependent join resolved a member")
	}
	if width := exact.SelectedWidth(); width != 0 {
		t.Fatalf("a rule that declares no dependent join sees selected width %d", width)
	}
}

// TestAFactorWithNoDestinationsStillHasCoordinates states the geometry half of
// the same separation. A Factor a rule selects members of but publishes no
// route into owns a coordinate universe and no destination universe, and that
// is a whole geometry rather than a broken one.
func TestAFactorWithNoDestinationsStillHasCoordinates(t *testing.T) {
	fixture := newSelectedFixture(t)
	table, tableOK := NewRouteTable(fixture.units, nil)
	if !tableOK {
		t.Fatal("a Factor with no destinations was refused a geometry")
	}
	if table.Routed() || table.Width() != selectedFixtureWidth {
		t.Fatalf("geometry routed=%t width=%d", table.Routed(), table.Width())
	}
	if _, resolved := table.routeMember(0, 0, 1); resolved {
		t.Fatal("a geometry with no destinations resolved a route")
	}
	member, resolved := table.selectedMember(0, 1)
	if !resolved || !member.Valid() || member.Routed() {
		t.Fatal("a geometry with no destinations resolved no observation")
	}
	if table, sealed := NewRouteTable(fixture.units, fixture.targets[:2]); !sealed || !table.Routed() {
		t.Fatal("an independently sized destination half was refused")
	}
}

// TestARoutedFoldRefusesAMemberItMayOnlyRead closes the loop. A member of a
// prior selection carries no destination, so a fold that staged one would be
// publishing at a coordinate its plan named only as something to read.
func TestARoutedFoldRefusesAMemberItMayOnlyRead(t *testing.T) {
	fixture := newSelectedFixture(t)
	write, writeOK := NewRouteWrite(fixture.binding, 0)
	if !writeOK {
		t.Fatal("route write")
	}
	cells, members, routes := routeCells(fixture, 2)
	readOnly := append([]RouteMember(nil), members...)
	readOnly[1] = RouteMember{coordinate: members[1].Coordinate()}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	if outcome := FoldSelectedRoute(ticket, write, &scratch, cells, readOnly, routes, routeLawReducer{empty: structure.NoSelection, failAt: -1}); outcome != structure.Refuse {
		t.Fatalf("a fold published through a member it may only read, settling %v", outcome)
	}
}

// TestAForeignSelectionResolvesMembersAtTheForeignAxisOwnTypes states how a
// dependent join on an axis this rule does not write is observed.
//
// The members are coordinates of the FOREIGN Factor, so they cannot come from
// this plane's own geometry - and the rule's family may not name that Factor's
// key and fact types either. The handle carries the geometry beside the read
// side it already carried, and the caller states the types for the same reason
// ForeignExactRead makes it state them: a handle asked for another pair is
// refused rather than reinterpreted.
func TestAForeignSelectionResolvesMembersAtTheForeignAxisOwnTypes(t *testing.T) {
	fixture := newSelectedFixture(t)
	table, tableOK := NewRouteTable(fixture.units, nil)
	if !tableOK {
		t.Fatal("foreign selection geometry")
	}
	foreign, foreignOK := NewForeignFactor(fixture.binding, table)
	if !foreignOK {
		t.Fatal("foreign handle")
	}
	member, resolved := ForeignSelectedMember[uint64, uint64](foreign, 1, 4)
	if !resolved || !member.Valid() {
		t.Fatal("a foreign axis resolved no member of its own coordinate universe")
	}
	if !member.Coordinate().Unit.Same(fixture.units[1]) || member.Tag() != 4 {
		t.Fatal("the foreign member does not name the coordinate its position holds")
	}
	if member.Routed() {
		t.Fatal("a foreign member carries a destination; a rule publishes into the Factor it writes")
	}
	if _, resolved := ForeignSelectedMember[uint32, uint32](foreign, 1, 4); resolved {
		t.Fatal("a foreign selection was resolved at another Factor's types")
	}
	if _, resolved := ForeignSelectedMember[uint64, uint64](foreign, selectedFixtureWidth, 4); resolved {
		t.Fatal("a coordinate outside the foreign universe resolved a member")
	}
	if width := ForeignSelectionWidth[uint64, uint64](foreign); width != selectedFixtureWidth {
		t.Fatalf("foreign selection width = %d, want %d", width, selectedFixtureWidth)
	}

	own := newSelectedFixture(t)
	stranger, strangerOK := NewForeignFactor(own.binding, table)
	if !strangerOK {
		t.Fatal("stranger handle")
	}
	if _, resolved := ForeignSelectedMember[uint64, uint64](stranger, 1, 4); resolved {
		t.Fatal("a coordinate another binding minted resolved against this one")
	}
}

// TestForeignSelectionIsNarrowerThanTheForeignReadTable states the fence.
// Enumerating the members of an axis is what a dependent join does; a rule
// that reads one coordinate of an axis has no member set of it to walk, so the
// selection handle is published for exactly the axes its plan selects and the
// read handle stays published for every axis its plan joins.
func TestForeignSelectionIsNarrowerThanTheForeignReadTable(t *testing.T) {
	fixture := newSelectedFixture(t)
	table, tableOK := NewRouteTable(fixture.units, fixture.targets)
	if !tableOK {
		t.Fatal("route geometry")
	}
	entries := make([]ForeignFactor, 3)
	for index := range entries {
		entry, entryOK := NewForeignFactor(fixture.binding, table)
		if !entryOK {
			t.Fatal("foreign entry")
		}
		entries[index] = entry
	}
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, table, entries, nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	exact, exactOK := plane.forRule([]FormRow{{Member: 0, Form: FormExact, Rule: planCompiledExactRule(t)}})
	if !exactOK {
		t.Fatal("narrowed exact plane")
	}
	if _, published := exact.Foreign(1); !published {
		t.Fatal("an axis this rule joins has no read handle")
	}
	if _, published := exact.ForeignSelection(1); published {
		t.Fatal("an axis this rule only reads one coordinate of published a selection handle")
	}

	routed, routedOK := plane.forRule([]FormRow{{Member: 0, Form: FormSelectedRoute, Rule: routedDescriptor(t, 1)}})
	if !routedOK {
		t.Fatal("narrowed routed plane")
	}
	if _, published := routed.ForeignSelection(2); !published {
		t.Fatal("the axis this rule selects published no selection handle")
	}
	if _, published := routed.ForeignSelection(0); published {
		t.Fatal("an axis no join of this rule named published a selection handle")
	}
}
