package infer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

// iterateSCCFixpoint runs fixpoint iteration for a single SCC until convergence.
// Returns true if types stabilized within the iteration limit.
func (i *Inferencer) iterateSCCFixpoint(
	run RunContext,
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) bool {
	for _, sym := range scc {
		if (len(summaries[sym]) == 0 || len(summaries[sym]) == 1 && typ.IsUnresolved(summaries[sym][0])) && i.recursive(sym) {
			summaries[sym] = returns.RecursionVariables(returnArity(localFuncs[sym]))
		}
	}
	for iter := 0; iter < i.maxIterations; iter++ {
		next, changed := i.runSCCIteration(run, scc, localFuncs, summaries)
		applySCCIterationUpdates(summaries, scc, next)
		if !changed {
			return true
		}
	}
	return false
}

func (i *Inferencer) planLocalFunctionSCCs(run RunContext, localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo) [][]cfg.SymbolID {
	// Propagate inter-procedural parameter hints across local call edges before
	// SCC return inference so unannotated params get stable callsite-driven seeds.
	var moduleBindings *bind.BindingTable
	if i != nil {
		if i.store != nil {
			moduleBindings = i.store.ModuleBindings()
		}
	}
	returns.PropagateParamHintsFromCallGraph(localFuncs, run.Env)
	adj := returns.BuildLocalCallGraph(localFuncs, moduleBindings, run.Env)
	return returns.ComputeSymbolSCCs(adj)
}

func seedSummariesFromSeed(
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	seed map[cfg.SymbolID][]typ.Type,
) map[cfg.SymbolID][]typ.Type {
	summaries := make(map[cfg.SymbolID][]typ.Type, len(localFuncs))
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		if seeded := seed[sym]; len(seeded) > 0 {
			summaries[sym] = seeded
		} else if info := localFuncs[sym]; info != nil && info.Fn != nil {
			// Only this SCC's missing return slot is an inference hole.
			summaries[sym] = []typ.Type{typ.Unresolved}
		}
	}
	return summaries
}

func (i *Inferencer) processSCCSummaries(
	run RunContext,
	sccs [][]cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) []diag.Diagnostic {
	var diags []diag.Diagnostic
	for _, scc := range sccs {
		if len(scc) == 0 {
			continue
		}
		if i.iterateSCCFixpoint(run, scc, localFuncs, summaries) {
			continue
		}
		if warn := i.widenSCCToUnknown(scc, localFuncs, summaries); warn != nil {
			diags = append(diags, *warn)
		}
	}
	for _, sym := range cfg.SortedSymbolIDs(summaries) {
		for slot, t := range summaries[sym] {
			summaries[sym][slot] = typ.Finalize(t)
		}
	}
	return diags
}

func (i *Inferencer) runSCCIteration(
	run RunContext,
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) (map[cfg.SymbolID][]typ.Type, bool) {
	changed := false
	next := make(map[cfg.SymbolID][]typ.Type, len(scc))
	for _, sym := range scc {
		info := localFuncs[sym]
		if info == nil || info.Fn == nil {
			continue
		}
		newReturn := i.inferReturnWithSummary(run, info, summaries, localFuncs)
		oldReturn := summaries[sym]
		merged := returns.MergeReturnSummary(oldReturn, newReturn)
		if i.recursive(sym) {
			newReturn = returns.TieRecursiveReturns(oldReturn, newReturn)
			merged = returns.AdvanceReturnSummary(oldReturn, newReturn)
		}
		next[sym] = merged
		if !returns.ReturnTypesEqual(merged, oldReturn) {
			changed = true
		}
	}
	return next, changed
}

func applySCCIterationUpdates(
	summaries map[cfg.SymbolID][]typ.Type,
	scc []cfg.SymbolID,
	next map[cfg.SymbolID][]typ.Type,
) {
	for _, sym := range scc {
		if v, ok := next[sym]; ok {
			summaries[sym] = v
		}
	}
}

// widenSCCToUnknown widens all SCC members to unknown when fixpoint did not converge.
// Preserves return arity while replacing type slots with unknown.
func (i *Inferencer) widenSCCToUnknown(
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) *diag.Diagnostic {
	for _, sym := range scc {
		summaries[sym] = typ.UnknownReturns(len(summaries[sym]))
	}
	if info := localFuncs[scc[0]]; info != nil && info.Fn != nil {
		return &diag.Diagnostic{
			Position: diag.Position{File: i.sourceName, Line: info.Fn.Line(), Column: info.Fn.Column()},
			Span:     ast.SpanOf(info.Fn),
			Severity: diag.SeverityWarning,
			Message:  "return type fixpoint did not converge; using unknown",
		}
	}
	return nil
}

// recursive reports whether the local function bound to sym can call itself.
func (i *Inferencer) recursive(sym cfg.SymbolID) bool {
	return returns.CallsItself(i.store, sym)
}

// returnArity returns the largest number of values a return statement of the
// function lists, at least one.
func returnArity(info *returns.LocalFuncInfo) int {
	arity := 1
	if info == nil || info.Graph == nil {
		return arity
	}
	info.Graph.EachReturn(func(_ cfg.Point, ret *cfg.ReturnInfo) {
		if ret != nil && len(ret.Exprs) > arity {
			arity = len(ret.Exprs)
		}
	})
	return arity
}
