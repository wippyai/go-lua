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

// RouteTable is one Factor's sealed dense route geometry: the exact Unit each
// dense coordinate is read at, and the strong Target it is written to, paired
// by position when the Factor binds. It is a value handoff of tables the
// Factor already owns - nothing here mints a coordinate - and it publishes no
// slice of its own, so the geometry has exactly one owner.
//
// Halves of unequal width are not a geometry at all: a coordinate readable at
// a Unit with no destination, or writable at a Target with no observation,
// would be a route only one half of a row could follow. Such a table is
// refused rather than truncated to the shorter half.
type RouteTable struct {
	units   []carrier.Unit
	targets []carrier.Target
}

// NewRouteTable seals one Factor's paired dense route geometry.
func NewRouteTable(units []carrier.Unit, targets []carrier.Target) (RouteTable, bool) {
	if len(units) != len(targets) {
		return RouteTable{}, false
	}
	return RouteTable{units: units, targets: targets}, true
}

// Width is the dense extent this geometry covers.
func (table RouteTable) Width() int { return len(table.units) }

// RouteMember is one member of a row's derived relation: the exact coordinate
// the member is observed at, paired with the strong target the member's
// reduced fact is published to, and carrying the owner-issued tag that names
// it within the row.
//
// Its halves are unexported because neither is a coordinate a caller may
// choose. Both are resolved from one dense position of one authenticated
// Factor geometry, so a member is either the whole route or nothing.
type RouteMember struct {
	coordinate SelectedCoordinate
	target     carrier.Target
}

// Coordinate is the member's read side: the sealed exact Unit and the tag.
func (member RouteMember) Coordinate() SelectedCoordinate { return member.coordinate }

// Tag is the owner-issued name of this member within its row.
func (member RouteMember) Tag() uint64 { return member.coordinate.Tag }

// Valid proves both halves of the member were resolved.
func (member RouteMember) Valid() bool {
	return member.coordinate.Unit != (carrier.Unit{}) && member.target != (carrier.Target{})
}

// routeMember resolves one dense position of this geometry into the pair, and
// refuses a position the geometry does not cover, a read side that is not an
// exact observation, or a write side that is not a strong publication.
func (table RouteTable) routeMember(dense uint32, tag uint64) (RouteMember, bool) {
	if uint64(dense) >= uint64(len(table.units)) {
		return RouteMember{}, false
	}
	unit := table.units[int(dense)]
	target := table.targets[int(dense)]
	if unit == (carrier.Unit{}) || unit.Kind() != carrier.ExactUnit {
		return RouteMember{}, false
	}
	if target == (carrier.Target{}) || target.Mode() != carrier.StrongTarget {
		return RouteMember{}, false
	}
	return RouteMember{coordinate: SelectedCoordinate{Unit: unit, Tag: tag}, target: target}, true
}

// RouteMember resolves one dense coordinate of this plane's own Factor into
// the read/write pair a routed row follows, and names it with the tag the
// row's derived relation issued.
//
// It is fenced twice. The plane must be the narrowed one an installer
// receives, and that rule must actually declare a routed publication over a
// selected join: a rule whose plan states no route has no route geometry to
// see, which is the same fence the foreign table is narrowed under. Then both
// halves are authenticated against this plane's own binding, so a coordinate
// another Factor minted resolves to nothing here rather than to a route.
func (plane FormPlane[K, V]) RouteMember(dense uint32, tag uint64) (RouteMember, bool) {
	if !plane.Valid() || !plane.routed {
		return RouteMember{}, false
	}
	member, memberOK := plane.routes.routeMember(dense, tag)
	if !memberOK {
		return RouteMember{}, false
	}
	if !plane.binding.ValidUnit(member.coordinate.Unit) || !plane.binding.ValidTarget(member.target) {
		return RouteMember{}, false
	}
	return member, true
}

// RouteWidth is the dense extent of the route geometry this rule may address.
// A family sizes its own member storage from it and never probes for the end
// of the table.
func (plane FormPlane[K, V]) RouteWidth() int {
	if !plane.Valid() || !plane.routed {
		return 0
	}
	return plane.routes.Width()
}
