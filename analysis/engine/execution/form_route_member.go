// form_route_member.go owns the one member of a dependent relation: the pair
// of where that member is READ and where it is WRITTEN.
//
// A J/WR row derives its relation once and then does two things with every
// member of it - observes the fact at that member's coordinate, and publishes
// the reduced fact at that member's destination. Those were two independently
// constructed vectors, which made "derive once" a discipline a fold could
// break by observing one member and publishing at another's destination. Here
// they are one value, minted together out of the Factor's own paired tables,
// so the relation a row derived is the relation both halves consume.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
)

// RouteTable is one Factor's sealed dense selection geometry: the exact Unit
// each read coordinate is observed at, and - for a Factor some rule publishes
// routes into - the strong Targets addressed by the output coordinate. The
// read and destination coordinates are separate dense spaces: a routed member
// resolves units[readDense] and targets[targetDense], never a positional pair
// chosen by the engine. Both tables are owner-issued and are authenticated by
// the binding when a FormPlane resolves a member.
//
// The destination half is optional and the observation half is not. A Factor
// a rule selects members OF but publishes no route INTO has coordinates and no
// destinations, which is an ordinary dependent join.
type RouteTable struct {
	units   []carrier.Unit
	targets []carrier.Target
}

// NewRouteTable seals one Factor's dense selection geometry. targets may be
// empty, which says this Factor publishes no route.
func NewRouteTable(units []carrier.Unit, targets []carrier.Target) (RouteTable, bool) {
	return RouteTable{units: units, targets: targets}, true
}

// Width is the dense extent this geometry covers.
func (table RouteTable) Width() int { return len(table.units) }

// Routed reports whether this geometry carries destinations at all.
func (table RouteTable) Routed() bool { return len(table.targets) != 0 }

// RouteMember is one member of a row's derived relation: the exact coordinate
// the member is observed at, carrying the owner-issued tag that names it
// within the row, and - when the row publishes routes - the strong target that
// member's reduced fact goes to.
//
// Its halves are unexported because neither is a coordinate a caller may
// choose. Both are resolved from the authenticated Factor geometry, so a
// routed member is the whole route or nothing, and a member of a dependent
// join that publishes nothing carries no destination to take apart.
type RouteMember struct {
	coordinate SelectedCoordinate
	target     carrier.Target
}

// Coordinate is the member's read side: the sealed exact Unit and the tag.
func (member RouteMember) Coordinate() SelectedCoordinate { return member.coordinate }

// Tag is the owner-issued name of this member within its row.
func (member RouteMember) Tag() uint64 { return member.coordinate.Tag }

// Valid proves the member names a coordinate to observe.
func (member RouteMember) Valid() bool {
	return member.coordinate.Unit != (carrier.Unit{})
}

// Routed proves the member also names the destination its fact publishes to.
func (member RouteMember) Routed() bool {
	return member.Valid() && member.target != (carrier.Target{})
}

// selectedMember resolves one dense position's observation side, and refuses a
// position the geometry does not cover or a coordinate that is not an exact
// observation.
func (table RouteTable) selectedMember(dense uint32, tag uint64) (RouteMember, bool) {
	if uint64(dense) >= uint64(len(table.units)) {
		return RouteMember{}, false
	}
	unit := table.units[int(dense)]
	if unit == (carrier.Unit{}) || unit.Kind() != carrier.ExactUnit {
		return RouteMember{}, false
	}
	return RouteMember{coordinate: SelectedCoordinate{Unit: unit, Tag: tag}}, true
}

// routeMember resolves one route's read and write coordinates independently.
// It refuses either side when the owner did not seal that dense coordinate or
// when the destination is absent or not a strong publication.
func (table RouteTable) routeMember(readDense, targetDense uint32, tag uint64) (RouteMember, bool) {
	member, memberOK := table.selectedMember(readDense, tag)
	if !memberOK || !table.Routed() {
		return RouteMember{}, false
	}
	if uint64(targetDense) >= uint64(len(table.targets)) {
		return RouteMember{}, false
	}
	target := table.targets[int(targetDense)]
	if target == (carrier.Target{}) || target.Mode() != carrier.StrongTarget {
		return RouteMember{}, false
	}
	member.target = target
	return member, true
}

// RouteMember resolves the read and write dense coordinates of this plane's
// own Factor into the pair a routed row follows, and names it with the tag the
// row's derived relation issued. The read coordinate supplies the support
// unit; the destination coordinate supplies the strong write target.
//
// It is fenced twice. The plane must be the narrowed one an installer
// receives, and that rule must actually declare a routed publication over a
// selected join: a rule whose plan states no route has no route geometry to
// see, which is the same fence the foreign table is narrowed under. Then both
// halves are authenticated against this plane's own binding, so a coordinate
// another Factor minted resolves to nothing here rather than to a route.
func (plane FormPlane[K, V]) RouteMember(readDense, targetDense uint32, tag uint64) (RouteMember, bool) {
	if !plane.Valid() || !plane.routed {
		return RouteMember{}, false
	}
	member, memberOK := plane.routes.routeMember(readDense, targetDense, tag)
	if !memberOK {
		return RouteMember{}, false
	}
	if !plane.binding.ValidUnit(member.coordinate.Unit) || !plane.binding.ValidTarget(member.target) {
		return RouteMember{}, false
	}
	return member, true
}

// SelectedMember resolves one dense coordinate of this plane's own Factor into
// the observation half alone, for a dependent join this rule reads and does
// not publish through.
//
// The fence is the rule's own plan: a rule that declares no selected read has
// no members to resolve, in the same way that a rule declaring no route has no
// destinations. What it never resolves is a destination - a member handed out
// here carries none, so a fold cannot publish through a join its plan did not
// name as the route.
func (plane FormPlane[K, V]) SelectedMember(dense uint32, tag uint64) (RouteMember, bool) {
	if !plane.Valid() || !plane.selects {
		return RouteMember{}, false
	}
	member, memberOK := plane.routes.selectedMember(dense, tag)
	if !memberOK || !plane.binding.ValidUnit(member.coordinate.Unit) {
		return RouteMember{}, false
	}
	return member, true
}

// SelectedWidth is the dense extent of the coordinate universe this rule may
// observe members of.
func (plane FormPlane[K, V]) SelectedWidth() int {
	if !plane.Valid() || !plane.selects {
		return 0
	}
	return plane.routes.Width()
}

// RouteWidth is the dense extent of the route geometry this rule may address.
// A family sizes its own member storage from it and never probes for the end
// of the table.
func (plane FormPlane[K, V]) RouteWidth() int {
	if !plane.Valid() || !plane.routed {
		return 0
	}
	return len(plane.routes.targets)
}
