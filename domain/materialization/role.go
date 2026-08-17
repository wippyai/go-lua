// Package materialization owns the one finite age vocabulary shared by
// domain facts that distinguish an exact, recent, or merged alternative.
// It has no structural identity or solver authority.
package materialization

// Role is the closed materialization age of an already-selected structural
// alternative. Invalid is never admitted into a domain fact.
type Role uint8

const (
	Invalid Role = iota
	Exact
	Recent
	Summary
)

// RoleCount is the size of the closed age catalog. The ordinals are dense
// from Exact, so a consumer indexes by role without a lookup.
const RoleCount = int(Summary)

// Valid reports whether role is one of the closed materialization ages.
func (role Role) Valid() bool { return role >= Exact && role <= Summary }

// Roles is the age catalog in ordinal order. It is the one enumeration of the
// vocabulary this package owns, so a consumer that visits every alternative
// projects it instead of restating the member list at its own site. The
// catalog is returned by value and costs no allocation to range over.
func Roles() [RoleCount]Role { return [RoleCount]Role{Exact, Recent, Summary} }

// RecentToSummary is the only age transition. Exact stays structural, and a
// Summary is already the absorbing image.
func RecentToSummary(role Role) (Role, bool) {
	if role != Recent {
		return Invalid, false
	}
	return Summary, true
}
