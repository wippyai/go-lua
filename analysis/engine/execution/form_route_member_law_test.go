package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
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
		member, resolved := plane.RouteMember(uint32(index), uint64(index)+1)
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
	if _, resolved := plane.RouteMember(selectedFixtureWidth, 1); resolved {
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
	if _, resolved := plane.RouteMember(0, 1); resolved {
		t.Fatal("an unnarrowed plane resolved a route member")
	}
	if width := plane.RouteWidth(); width != 0 {
		t.Fatalf("an unnarrowed plane published route width %d", width)
	}
	exact, exactOK := plane.forRule([]FormRow{{Member: 0, Form: FormExact, Rule: planCompiledExactRule(t)}})
	if !exactOK {
		t.Fatal("narrowed exact plane")
	}
	if _, resolved := exact.RouteMember(0, 1); resolved {
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
		if _, resolved := narrowed.RouteMember(uint32(index), 1); resolved {
			t.Fatalf("dense %d of a foreign geometry resolved as this Factor's route", index)
		}
	}
}

// TestRouteGeometryHasNoHalves states the pairing invariant at the seal. A
// coordinate readable with no destination, or writable with no observation, is
// a route only one half of a row could follow, so the two universes are either
// the same universe or there is no geometry at all.
func TestRouteGeometryHasNoHalves(t *testing.T) {
	fixture := newSelectedFixture(t)
	if _, sealed := NewRouteTable(fixture.units, fixture.targets[:2]); sealed {
		t.Fatal("a geometry with fewer destinations than coordinates was sealed")
	}
	if _, sealed := NewRouteTable(fixture.units[:2], fixture.targets); sealed {
		t.Fatal("a geometry with fewer coordinates than destinations was sealed")
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
	member, resolved := plane.RouteMember(0, 1)
	if !resolved {
		t.Fatal("route member")
	}
	member.coordinate = SelectedCoordinate{Unit: fixture.units[1], Tag: 99}
	member.target = fixture.targets[1]
	again, resolvedAgain := plane.RouteMember(0, 1)
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
	cells, members := routeCells(fixture, 3)

	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	var scratch RouteScratch[uint64, uint64]
	shifted := []RouteMember{members[1], members[2], members[0]}
	if outcome := FoldSelectedRoute(ticket, write, &scratch, cells, shifted, routeLawReducer{empty: structure.NoSelection, failAt: -1}); outcome != structure.Refuse {
		t.Fatalf("a row that published a second member set settled %v, want refuse", outcome)
	}

	narrow := NewRun(1, 1)
	narrowTicket := issueSelected(t, narrow, fixture, fixture.state)
	var narrowScratch RouteScratch[uint64, uint64]
	if outcome := FoldSelectedRoute(narrowTicket, write, &narrowScratch, cells, members[:2], routeLawReducer{empty: structure.NoSelection, failAt: -1}); outcome != structure.Refuse {
		t.Fatalf("a row with fewer destinations than observations settled %v, want refuse", outcome)
	}

	whole := NewRun(1, 1)
	wholeTicket := issueSelected(t, whole, fixture, fixture.state)
	var wholeScratch RouteScratch[uint64, uint64]
	if outcome := FoldSelectedRoute(wholeTicket, write, &wholeScratch, cells, members, routeLawReducer{empty: structure.NoSelection, failAt: -1}); outcome != structure.Concrete {
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
	if _, resolved := narrowed.RouteMember(1, 1); resolved {
		t.Fatal("a coordinate with no exact observation resolved a route")
	}
	if _, resolved := narrowed.RouteMember(2, 1); resolved {
		t.Fatal("a coordinate with no strong destination resolved a route")
	}
	if _, resolved := narrowed.RouteMember(0, 1); !resolved {
		t.Fatal("a whole geometry row resolved no route")
	}
}
