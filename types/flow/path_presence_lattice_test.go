package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/lattice"
)

// pathPresenceName renders a pathPresence value for diagnostic messages so the
// LawSuite reports a meaningful element name on violation instead of a raw
// uint8.
func pathPresenceName(p pathPresence) string {
	switch p {
	case pathPresenceUnknown:
		return "Unknown"
	case pathPresencePresent:
		return "Present"
	case pathPresenceAbsent:
		return "Absent"
	case pathPresenceMaybe:
		return "Maybe"
	default:
		return "Invalid"
	}
}

// pathPresenceSample is the exhaustive sample over the 4-element carrier.
// Per PRESENCE_DOMAIN_DESIGN.md §7 (rev 2): the carrier is finite, so
// 4-element exhaustive coverage is the complete law check.
var pathPresenceSample = []pathPresence{
	pathPresenceUnknown,
	pathPresencePresent,
	pathPresenceAbsent,
	pathPresenceMaybe,
}

// TestPathPresenceLattice_Laws applies lattice.LawSuite to the PathPresence
// domain across the entire 4-element carrier (exhaustive).
//
// PRESENCE_DOMAIN_DESIGN.md §10 acceptance: LawSuite passes on the carrier.
func TestPathPresenceLattice_Laws(t *testing.T) {
	suite := lattice.LawSuite[pathPresence]{
		Name:   "flow.pathPresence",
		Domain: pathPresenceDomain,
		Sample: pathPresenceSample,
		Format: pathPresenceName,
	}
	suite.Run(t)
}

// TestPathPresenceLattice_JoinTable pins the exact 4×4 = 16 entries of the
// join table per PRESENCE_DOMAIN_DESIGN.md §5 rev 2 (Codex amendment):
//
//	Unknown ⊔ x       = x
//	Maybe   ⊔ x       = Maybe
//	Present ⊔ Present = Present
//	Absent  ⊔ Absent  = Absent
//	Present ⊔ Absent  = Maybe
//
// Pinning the full table catches table-drift bugs that the algebraic laws
// would only catch indirectly (e.g. join commutativity would still hold if
// both entries flipped the same way).
func TestPathPresenceLattice_JoinTable(t *testing.T) {
	cases := []struct {
		a, b pathPresence
		want pathPresence
	}{
		// Unknown is the join identity (Bottom).
		{pathPresenceUnknown, pathPresenceUnknown, pathPresenceUnknown},
		{pathPresenceUnknown, pathPresencePresent, pathPresencePresent},
		{pathPresenceUnknown, pathPresenceAbsent, pathPresenceAbsent},
		{pathPresenceUnknown, pathPresenceMaybe, pathPresenceMaybe},

		// Present row.
		{pathPresencePresent, pathPresenceUnknown, pathPresencePresent},
		{pathPresencePresent, pathPresencePresent, pathPresencePresent},
		{pathPresencePresent, pathPresenceAbsent, pathPresenceMaybe},
		{pathPresencePresent, pathPresenceMaybe, pathPresenceMaybe},

		// Absent row.
		{pathPresenceAbsent, pathPresenceUnknown, pathPresenceAbsent},
		{pathPresenceAbsent, pathPresencePresent, pathPresenceMaybe},
		{pathPresenceAbsent, pathPresenceAbsent, pathPresenceAbsent},
		{pathPresenceAbsent, pathPresenceMaybe, pathPresenceMaybe},

		// Maybe is the absorbing element (Top).
		{pathPresenceMaybe, pathPresenceUnknown, pathPresenceMaybe},
		{pathPresenceMaybe, pathPresencePresent, pathPresenceMaybe},
		{pathPresenceMaybe, pathPresenceAbsent, pathPresenceMaybe},
		{pathPresenceMaybe, pathPresenceMaybe, pathPresenceMaybe},
	}
	for _, c := range cases {
		got := joinPathPresence(c.a, c.b)
		if got != c.want {
			t.Errorf("joinPathPresence(%s, %s) = %s, want %s",
				pathPresenceName(c.a), pathPresenceName(c.b),
				pathPresenceName(got), pathPresenceName(c.want))
		}
	}
}

// TestPathPresenceLattice_MeetTable pins the exact 4×4 = 16 entries of the
// meet table per PRESENCE_DOMAIN_DESIGN.md §5 rev 2 (Codex amendment):
//
//	Unknown ⊓ x       = Unknown
//	Maybe   ⊓ x       = x
//	Present ⊓ Present = Present
//	Absent  ⊓ Absent  = Absent
//	Present ⊓ Absent  = Unknown
func TestPathPresenceLattice_MeetTable(t *testing.T) {
	cases := []struct {
		a, b pathPresence
		want pathPresence
	}{
		// Unknown is Bottom (meet absorbs).
		{pathPresenceUnknown, pathPresenceUnknown, pathPresenceUnknown},
		{pathPresenceUnknown, pathPresencePresent, pathPresenceUnknown},
		{pathPresenceUnknown, pathPresenceAbsent, pathPresenceUnknown},
		{pathPresenceUnknown, pathPresenceMaybe, pathPresenceUnknown},

		// Present row.
		{pathPresencePresent, pathPresenceUnknown, pathPresenceUnknown},
		{pathPresencePresent, pathPresencePresent, pathPresencePresent},
		{pathPresencePresent, pathPresenceAbsent, pathPresenceUnknown},
		{pathPresencePresent, pathPresenceMaybe, pathPresencePresent},

		// Absent row.
		{pathPresenceAbsent, pathPresenceUnknown, pathPresenceUnknown},
		{pathPresenceAbsent, pathPresencePresent, pathPresenceUnknown},
		{pathPresenceAbsent, pathPresenceAbsent, pathPresenceAbsent},
		{pathPresenceAbsent, pathPresenceMaybe, pathPresenceAbsent},

		// Maybe is Top (meet is identity).
		{pathPresenceMaybe, pathPresenceUnknown, pathPresenceUnknown},
		{pathPresenceMaybe, pathPresencePresent, pathPresencePresent},
		{pathPresenceMaybe, pathPresenceAbsent, pathPresenceAbsent},
		{pathPresenceMaybe, pathPresenceMaybe, pathPresenceMaybe},
	}
	for _, c := range cases {
		got := meetPathPresence(c.a, c.b)
		if got != c.want {
			t.Errorf("meetPathPresence(%s, %s) = %s, want %s",
				pathPresenceName(c.a), pathPresenceName(c.b),
				pathPresenceName(got), pathPresenceName(c.want))
		}
	}
}

// TestPathPresenceLattice_JoinUnknownIsIdentity pins the specific polarity
// invariant Codex flagged as a blocker in rev 1: Unknown is the join identity,
// NOT a join-absorbing element. That is, joinPathPresence(Unknown, Present)
// must return Present (learning the information), not Unknown (discarding it).
//
// This is the "Unknown = Bottom" reading of the operational semantics — the
// only reading consistent with how presence is propagated through joins in
// the flow solver.
func TestPathPresenceLattice_JoinUnknownIsIdentity(t *testing.T) {
	cases := []struct {
		other pathPresence
	}{
		{pathPresenceUnknown},
		{pathPresencePresent},
		{pathPresenceAbsent},
		{pathPresenceMaybe},
	}
	for _, c := range cases {
		if got := joinPathPresence(pathPresenceUnknown, c.other); got != c.other {
			t.Errorf("joinPathPresence(Unknown, %s) = %s, want %s (Unknown is the join identity)",
				pathPresenceName(c.other), pathPresenceName(got), pathPresenceName(c.other))
		}
		if got := joinPathPresence(c.other, pathPresenceUnknown); got != c.other {
			t.Errorf("joinPathPresence(%s, Unknown) = %s, want %s (Unknown is the join identity)",
				pathPresenceName(c.other), pathPresenceName(got), pathPresenceName(c.other))
		}
	}
}

// TestPathPresenceLattice_WidenIsJoin pins the finite-height widening: per
// PRESENCE_DOMAIN_DESIGN.md §5 rev 2, Widen = Join. The 4-element lattice has
// height 2, so the trivial widening stabilizes any ascending chain in ≤ 2
// steps; no Cousot extrapolation is required.
func TestPathPresenceLattice_WidenIsJoin(t *testing.T) {
	for _, a := range pathPresenceSample {
		for _, b := range pathPresenceSample {
			w := pathPresenceDomain.Widen(a, b)
			j := joinPathPresence(a, b)
			if w != j {
				t.Errorf("Widen(%s, %s) = %s, want Join(%s, %s) = %s",
					pathPresenceName(a), pathPresenceName(b), pathPresenceName(w),
					pathPresenceName(a), pathPresenceName(b), pathPresenceName(j))
			}
		}
	}
}

// TestPathPresenceLattice_BottomAndTopDistinct pins the lattice
// non-degeneracy: Bottom and Top are distinct elements, Bottom ⊑ Top, and
// Top is not below Bottom. This catches the rev 1 polarity confusion (where
// "Unknown = Top" would have made Bottom and Top collapse onto the same
// element under the existing joinPathPresence semantics).
func TestPathPresenceLattice_BottomAndTopDistinct(t *testing.T) {
	bot := pathPresenceDomain.Bottom()
	top := pathPresenceDomain.Top()

	if pathPresenceDomain.Equal(bot, top) {
		t.Fatalf("Bottom (%s) and Top (%s) must be distinct elements",
			pathPresenceName(bot), pathPresenceName(top))
	}
	if !pathPresenceDomain.LessOrEq(bot, top) {
		t.Errorf("Bottom ⊑ Top fails: LessOrEq(%s, %s) = false",
			pathPresenceName(bot), pathPresenceName(top))
	}
	if pathPresenceDomain.LessOrEq(top, bot) {
		t.Errorf("Top ⊑ Bottom must NOT hold: LessOrEq(%s, %s) = true",
			pathPresenceName(top), pathPresenceName(bot))
	}

	if bot != pathPresenceUnknown {
		t.Errorf("Bottom = %s, want Unknown", pathPresenceName(bot))
	}
	if top != pathPresenceMaybe {
		t.Errorf("Top = %s, want Maybe", pathPresenceName(top))
	}
}
