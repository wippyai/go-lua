// Package execution owns the small, engine-private execution substrate used
// by typed generated lanes.  It carries lifecycle authority only; semantic
// values remain in the typed factbinding plane.
package execution

// Outcome is the complete disposition of one execution attempt.
//
// The zero value is Refuse.  The remaining values are intentionally fixed:
// callers cannot manufacture a value-bearing result by copying a semantic
// payload into an outcome.
type Outcome uint8

const (
	Refuse Outcome = iota
	NoSelection
	NoCandidate
	Concrete
	AuthenticatedOpaque
)

// Valid reports whether outcome is one of the five sealed dispositions.
func (outcome Outcome) Valid() bool {
	return outcome <= AuthenticatedOpaque
}
