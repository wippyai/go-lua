package callsite

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/types/typ"
)

// SymbolTypeAtPoint resolves a symbol type at a CFG point.
type SymbolTypeAtPoint func(cfg.Point, cfg.SymbolID) (typ.Type, bool)

// PreAssignmentTargets owns the call-site target symbols that must read
// predecessor state under Lua RHS-before-LHS assignment semantics.
type PreAssignmentTargets map[*cfg.CallInfo]functionsymbols.Set

// Contains reports whether call writes sym after evaluating its arguments.
func (t PreAssignmentTargets) Contains(call *cfg.CallInfo, sym cfg.SymbolID) bool {
	if call == nil || sym == 0 || len(t) == 0 {
		return false
	}
	targets, ok := t[call]
	return ok && targets.Contains(sym)
}

// PreAssignmentTargetsByCall indexes assignment target symbols by source call.
// For calls used as assignment RHS at point p (x = f(...)), the returned target
// set captures symbols that must be typed from predecessor state when computing
// argument evidence at that call site.
func PreAssignmentTargetsByCall(assignments []api.AssignmentEvidence) PreAssignmentTargets {
	if len(assignments) == 0 {
		return nil
	}
	out := make(PreAssignmentTargets)
	for _, assign := range assignments {
		info := assign.Info
		if info == nil || len(info.Targets) == 0 || len(info.SourceCalls) == 0 {
			continue
		}
		var targets functionsymbols.Set
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				targets.Add(target.Symbol)
			}
		}
		if targets.IsEmpty() {
			continue
		}
		for _, call := range info.SourceCalls {
			if call == nil {
				continue
			}
			existing := out[call]
			for _, sym := range targets.Slice() {
				existing.Add(sym)
			}
			out[call] = existing
		}
	}
	return out
}

// PreAssignmentTypeAtJoin returns the pre-assignment type for a symbol at point p.
//
// It joins symbol types across predecessor points only, preserving Lua RHS-before-LHS
// semantics for assignments such as `x = f(x)` without reading assignment-point state.
func PreAssignmentTypeAtJoin(
	graph *cfg.Graph,
	p cfg.Point,
	sym cfg.SymbolID,
	resolve SymbolTypeAtPoint,
) typ.Type {
	if graph == nil || sym == 0 || resolve == nil {
		return nil
	}

	var joined typ.Type
	for _, pred := range graph.Predecessors(p) {
		if t, ok := resolve(pred, sym); ok && t != nil {
			if joined == nil {
				joined = t
			} else {
				joined = typ.JoinPreferNonSoft(joined, t)
			}
		}
	}
	if joined != nil {
		return joined
	}
	return nil
}

// PreAssignmentTypeAtJoinOrPoint joins predecessor types first and falls back
// to point p when no predecessor value is available.
func PreAssignmentTypeAtJoinOrPoint(
	graph *cfg.Graph,
	p cfg.Point,
	sym cfg.SymbolID,
	resolve SymbolTypeAtPoint,
) typ.Type {
	if t := PreAssignmentTypeAtJoin(graph, p, sym, resolve); t != nil {
		return t
	}
	if resolve == nil {
		return nil
	}
	if t, ok := resolve(p, sym); ok && t != nil {
		return t
	}
	return nil
}
