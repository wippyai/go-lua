// Package change is the engine's single change-fact vocabulary. It has no
// dependencies, so every sealing owner can mint into it without coupling the
// planes it serves: equation and carrier are disjoint, and demand is their one
// join point.
package change

// Ord is a dense position inside one sealed plane, issued by that plane's seal
// and stamped into the row. Readers never re-derive it from a key.
type Ord uint32

// NoOrd names the absence of a position. A plane never issues it.
const NoOrd Ord = ^Ord(0)

// Reason is the closed change vocabulary. The values are bits so that evidence
// for one ordinal accumulates instead of the first arrival winning.
type Reason uint8

const (
	ChangedUnit Reason = 1 << iota
	ChangedFactor
	SupportAdded
	SupportRemoved
	AuthorshipChanged
)

// ReasonWidth is the width of the closed vocabulary. It is the length of a
// per-reason histogram and the bound of ReasonAt. Histogram position i holds
// the reason ReasonAt(i) names, so the order is the declaration order above.
const ReasonWidth = 5

// ReasonAt returns the reason occupying histogram position index.
func ReasonAt(index int) (Reason, bool) {
	if index < 0 || index >= ReasonWidth {
		return 0, false
	}
	return Reason(1) << uint(index), true
}

// Direction is the lattice-order fact carried alongside the reasons. It is
// three independent bits, not an enum: one operation can grow one axis and
// shrink another, and a producer that cannot decide must say so.
type Direction uint8

const (
	// Known says the producer classified this delta against the lattice order.
	Known Direction = 1 << iota
	// Ascends says some axis grew.
	Ascends
	// Descends says some axis shrank.
	Descends
)

// Set is accumulated evidence for one ordinal: what changed, and which way the
// change moved the lattice.
type Set struct {
	Reasons   Reason
	Direction Direction
}

// Has reports whether every reason bit in r is present.
func (s Set) Has(r Reason) bool { return s.Reasons&r == r }

// With adds reason bits without touching the direction axis.
func (s Set) With(r Reason) Set { s.Reasons |= r; return s }

// Empty reports the absence of any evidence at all.
func (s Set) Empty() bool { return s.Reasons == 0 && s.Direction == 0 }

// Admits is the single admissibility predicate for accumulator reuse. It is
// fail-closed by construction: the zero Set has Known unset, so an operation
// that reaches a consumer without classifying itself is refused, never
// admitted.
func (s Set) Admits() bool { return s.Direction&Known != 0 && s.Direction&Descends == 0 }

// Unknown reports that no contributor classified this evidence.
func (s Set) Unknown() bool { return s.Direction&Known == 0 }

// Union accumulates two evidence sets. Reasons OR. On the direction axis
// Known is conjunctive -- a set is classified only if every contributor was --
// while Ascends and Descends are disjunctive. The union of a classified ascent
// with an unclassified delta is therefore unclassified, and Admits refuses it.
func (s Set) Union(o Set) Set {
	return Set{
		Reasons:   s.Reasons | o.Reasons,
		Direction: (s.Direction & o.Direction & Known) | ((s.Direction | o.Direction) & (Ascends | Descends)),
	}
}
