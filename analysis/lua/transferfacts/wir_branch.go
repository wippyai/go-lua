package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

// directBranchCheckFromWIR returns the WIR-owned direct check for a branch
// point. Compound conditions still use the semantic sidecar because their
// implication/frozen-table facts depend on AST structure until the transfer
// interpreter owns that derivation.
func (l *lowerer) directBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	return l.firstDirectBranchCheckFromWIR(point)
}

func (l *lowerer) directBranchCheckAt(point cfg.Point, result *semantics.Result) (branchcond.Check, bool) {
	if check, ok := l.firstDirectBranchCheckFromWIR(point); ok {
		return check, true
	}
	if l != nil && l.wir != nil {
		return branchcond.Check{}, false
	}
	if result == nil {
		return branchcond.Check{}, false
	}
	fact, ok := result.BranchCondition(point)
	if !ok || fact.Check.Kind == branchcond.CheckNone {
		return branchcond.Check{}, false
	}
	return fact.Check, true
}

func (l *lowerer) firstDirectBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	if l == nil || l.wir == nil {
		return branchcond.Check{}, false
	}
	var out branchcond.Check
	var found bool
	l.wir.ForEachBranchCheck(point, func(check wir.Check) bool {
		candidate := branchCheckFromWIR(check)
		if candidate.Kind == branchcond.CheckNone {
			return true
		}
		out = candidate
		found = true
		return false
	})
	return out, found
}

func branchCheckFromWIR(check wir.Check) branchcond.Check {
	return branchcond.Check{
		Kind:          branchcond.CheckKind(check.Kind),
		Path:          check.Path,
		OtherPath:     check.OtherPath,
		TypeName:      check.TypeName,
		Literal:       check.Literal,
		LiteralString: check.LiteralString,
		LenFloor:      check.LenFloor,
		NumFloor:      check.NumFloor,
		Negated:       check.Negated,
	}
}
