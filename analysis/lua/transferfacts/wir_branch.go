package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// directBranchCheckFromWIR returns the WIR-owned direct check for a branch
// point. Compound conditions still use the semantic sidecar because their
// implication/frozen-table facts depend on AST structure until the transfer
// interpreter owns that derivation.
func (l *lowerer) directBranchCheckFromWIR(point cfg.Point, fallback branchcond.Check) (branchcond.Check, bool) {
	if l.wir == nil || fallback.Kind == branchcond.CheckNone {
		return branchcond.Check{}, false
	}
	var out branchcond.Check
	var found bool
	l.wir.ForEachBranchCheck(point, func(check wir.Check) bool {
		candidate := branchCheckFromWIR(check)
		if !branchChecksEqual(candidate, fallback) {
			return true
		}
		out = candidate
		found = true
		return false
	})
	return out, found
}

func (l *lowerer) directBranchCheckAt(point cfg.Point, result *semantics.Result) (branchcond.Check, bool) {
	if l.wir != nil {
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
		if found {
			return out, true
		}
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

func branchChecksEqual(a, b branchcond.Check) bool {
	if a.Kind != b.Kind {
		return false
	}
	if !a.Path.Equal(b.Path) || !a.OtherPath.Equal(b.OtherPath) {
		return false
	}
	if a.TypeName != b.TypeName || a.LiteralString != b.LiteralString {
		return false
	}
	if a.LenFloor != b.LenFloor || a.NumFloor != b.NumFloor || a.Negated != b.Negated {
		return false
	}
	if a.Literal == nil || b.Literal == nil {
		return a.Literal == nil && b.Literal == nil
	}
	return typ.TypeEquals(a.Literal, b.Literal)
}
