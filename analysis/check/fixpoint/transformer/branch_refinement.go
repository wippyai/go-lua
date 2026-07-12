package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
)

// BranchPathValueTerm resolves the current symbolic value of one factflow path.
// The resolver is also the structural-safety proof: returning false means the
// path cannot be represented without concrete visibility/heap semantics.
type BranchPathValueTerm func(pathdom.Path) (ValueTerm, bool)

// SymbolicBranchRefinement is one ordered path-value update. TargetPathRef is
// borrowed from immutable factflow storage and must not be mutated or retained.
type SymbolicBranchRefinement struct {
	target pathdom.Path
	value  ValueTerm
}

func (r SymbolicBranchRefinement) TargetPathRef() pathdom.Path { return r.target }
func (r SymbolicBranchRefinement) Value() ValueTerm            { return r.value }

// LowerBranchRefinements retains the concrete shallow-before-deep order from
// BranchAlgebra and lowers positive product constraints through Arena.RefineValue.
// Persistent evidence, path relations, and sufficient-literal implications
// remain contextual: publishing only the scalar narrowing would be unsound.
func LowerBranchRefinements(arena *Arena, branch factapply.BranchAlgebra, cond bool, resolve BranchPathValueTerm) ([]SymbolicBranchRefinement, error) {
	if arena == nil || resolve == nil {
		return nil, fmt.Errorf("branch: arena and path resolver are required")
	}
	for _, reason := range branch.GuardOnlyBlockers() {
		switch reason {
		case "branch:missing-condition-source", "branch:path-relation", "branch:path-evidence", "branch:sufficient-literal-case":
			return nil, fmt.Errorf("%s", reason)
		}
	}
	active := branch.ActiveRefinements(cond)
	out := make([]SymbolicBranchRefinement, 0, len(active))
	for _, refinement := range active {
		target := refinement.TargetPathRef()
		value, ok := latestSymbolicPathValue(out, target)
		if !ok {
			value, ok = resolve(target)
		}
		if !ok || value == 0 {
			return nil, fmt.Errorf("branch: contextual-refinement-path")
		}
		value, ok = arena.RefineValue(value, refinement.Refinement())
		if !ok {
			return nil, fmt.Errorf("branch: contextual-refinement-kind")
		}
		out = append(out, SymbolicBranchRefinement{target: target, value: value})
	}
	return out, nil
}

func latestSymbolicPathValue(updates []SymbolicBranchRefinement, target pathdom.Path) (ValueTerm, bool) {
	for i := len(updates) - 1; i >= 0; i-- {
		if updates[i].target.Equal(target) {
			return updates[i].value, true
		}
	}
	return 0, false
}

// BranchRefinementKernelExact reports whether every edge refinement uses the
// positive scalar kernel. It intentionally ignores other branch families so a
// census can distinguish refinement capability gained from whole-branch
// eligibility, which remains blocked until evidence and relations are lowered.
func BranchRefinementKernelExact(branch factapply.BranchAlgebra) bool {
	for _, cond := range []bool{false, true} {
		for _, active := range branch.ActiveRefinements(cond) {
			refinement := active.Refinement()
			if refinement.NegatedLiteral() || refinement.FalsyAbsent() {
				return false
			}
			if _, ok := refinement.Constraint(); !ok {
				return false
			}
		}
	}
	return true
}
