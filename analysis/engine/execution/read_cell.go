// read_cell.go owns the materialization half of the read-boundary contract: the
// one place an observed Factor cell becomes a delivered cell. It sits below both
// consumers - the engine's staged read boundary and the generated execution
// forms - so the sparse-default substitution and the opaque widening are stated
// exactly once and a form never restates engine policy.

package execution

// ReadCellPolicy carries one read's sealed substitutions. Its zero value
// delivers every observed coordinate unchanged, which is the reading of a
// contract that declares explicit sparsity and refusal on opacity.
type ReadCellPolicy[V any] struct {
	defaulted bool
	fallback  V
	widened   bool
	top       V
}

// NewReadCellPolicy seals the substitutions a read's declared contract derived
// from its Factor: fallback is delivered at an unwritten coordinate when the
// read declares the Factor default, and top is what every coordinate becomes
// once the read is widened.
func NewReadCellPolicy[V any](defaulted bool, fallback V, top V) ReadCellPolicy[V] {
	return ReadCellPolicy[V]{defaulted: defaulted, fallback: fallback, top: top}
}

// Cell delivers one observed coordinate under the sealed substitutions.
// Widening dominates: a read whose alternative set is opaque delivers the
// Factor's Top at every coordinate, which is the sound over-approximation of
// any value the unobserved alternative could have written.
func (policy ReadCellPolicy[V]) Cell(value V, present bool) (V, bool) {
	switch {
	case policy.widened:
		return policy.top, true
	case policy.defaulted && !present:
		return policy.fallback, true
	}
	return value, present
}

// Widen returns the policy this read uses for a source row whose locator
// reported an opaque alternative.
func (policy ReadCellPolicy[V]) Widen() ReadCellPolicy[V] {
	policy.widened = true
	return policy
}
