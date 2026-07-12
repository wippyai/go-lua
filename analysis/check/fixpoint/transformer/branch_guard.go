package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

// BranchValueTerm resolves the canonical concrete condition source into the
// current relation arena. Resolution owns boundary/path identity; guard
// lowering owns only Lua truthy/falsy polarity.
type BranchValueTerm func(factflow.ValueSource) (ValueTerm, bool)

// LowerBranchGuards lowers the initial exact guard-only slice. The branch view
// is the same factapply algebra used by concrete edge execution. Any active
// refinement, relation, persistent evidence, or sufficient-literal implication
// fails closed until a symbolic output transaction represents that family.
func LowerBranchGuards(arena *Arena, branch factapply.BranchAlgebra, resolve BranchValueTerm) (truthy, falsy Guard, err error) {
	if arena == nil || resolve == nil {
		return 0, 0, fmt.Errorf("branch: arena and condition resolver are required")
	}
	if exact, reason := branch.GuardOnly(); !exact {
		return 0, 0, fmt.Errorf("%s", reason)
	}
	source, _ := branch.ConditionSource()
	value, ok := resolve(source)
	if !ok || value == 0 {
		return 0, 0, fmt.Errorf("branch: contextual-condition-source")
	}
	return arena.Truthy(value), arena.Falsy(value), nil
}
