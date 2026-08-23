package static

import "github.com/wippyai/go-lua/analysis/schema/structure"

// IdentityTypeFact is Static's direct identity reducer. TypeFact is already
// an owner-issued lattice cell, so the reducer copies the exact input and
// concludes concretely; the owner fence stays with the TypeFact algebra.
func IdentityTypeFact(input TypeFact) (TypeFact, structure.ReductionOutcome) {
	return input, structure.Concrete
}
