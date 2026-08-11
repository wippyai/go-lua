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

// Valid reports whether role is one of the closed materialization ages.
func (role Role) Valid() bool { return role >= Exact && role <= Summary }

// RecentToSummary is the only age transition. Exact stays structural, and a
// Summary is already the absorbing image.
func RecentToSummary(role Role) (Role, bool) {
	if role != Recent {
		return Invalid, false
	}
	return Summary, true
}
