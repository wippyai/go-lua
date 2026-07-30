package transformer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// SymbolicBranchRefinement is one ordered path-value update. TargetPathRef is
// borrowed from immutable factflow storage and must not be mutated or retained.
type SymbolicBranchRefinement struct {
	target pathdom.Path
	value  ValueTerm
}

func (r SymbolicBranchRefinement) TargetPathRef() pathdom.Path { return r.target }
func (r SymbolicBranchRefinement) Value() ValueTerm            { return r.value }

func latestSymbolicPathValue(updates []SymbolicBranchRefinement, target pathdom.Path) (ValueTerm, bool) {
	for i := len(updates) - 1; i >= 0; i-- {
		if updates[i].target.Equal(target) {
			return updates[i].value, true
		}
	}
	return 0, false
}
