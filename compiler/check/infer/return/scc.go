package infer

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

// iterateSCCFixpoint runs fixpoint iteration for a single SCC until convergence.
// Returns once the widened return-vector product stabilizes.
func (i *Inferencer) iterateSCCFixpoint(
	run RunContext,
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	returnVectors map[cfg.SymbolID][]typ.Type,
) bool {
	for {
		next, changed := i.runSCCIteration(run, scc, localFuncs, returnVectors)
		applySCCIterationUpdates(returnVectors, scc, next)
		if !changed {
			return true
		}
	}
}

func (i *Inferencer) planLocalFunctionSCCs(localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo) [][]cfg.SymbolID {
	// Propagate inter-procedural parameter evidence across local call edges before
	// SCC return inference so unannotated params get stable callsite-driven seeds.
	returns.PropagateParameterEvidence(localFuncs)
	projectLocalParameterEvidence(localFuncs)

	var moduleBindings *bind.BindingTable
	if i != nil && i.store != nil {
		moduleBindings = i.store.ModuleBindings()
	}
	adj := returns.BuildLocalCallGraph(localFuncs, moduleBindings)
	return returns.ComputeSymbolSCCs(adj)
}

func projectLocalParameterEvidence(localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo) {
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		info := localFuncs[sym]
		if info == nil || len(info.ParameterEvidence) == 0 {
			continue
		}
		info.ParameterEvidence = paramevidence.ProjectToParameterUse(info.Graph, info.Fn, info.ParameterEvidence)
	}
}

func seedReturnVectorsFromSeed(
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	seed map[cfg.SymbolID][]typ.Type,
) map[cfg.SymbolID][]typ.Type {
	returnVectors := make(map[cfg.SymbolID][]typ.Type, len(localFuncs))
	if seed == nil {
		return returnVectors
	}
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		if seeded := seed[sym]; len(seeded) > 0 {
			returnVectors[sym] = seeded
		}
	}
	return returnVectors
}

func (i *Inferencer) processSCCReturnVectors(
	run RunContext,
	sccs [][]cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	returnVectors map[cfg.SymbolID][]typ.Type,
) []diag.Diagnostic {
	for _, scc := range sccs {
		if len(scc) == 0 {
			continue
		}
		i.iterateSCCFixpoint(run, scc, localFuncs, returnVectors)
	}
	return nil
}

func (i *Inferencer) runSCCIteration(
	run RunContext,
	scc []cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	returnVectors map[cfg.SymbolID][]typ.Type,
) (map[cfg.SymbolID][]typ.Type, bool) {
	changed := false
	next := make(map[cfg.SymbolID][]typ.Type, len(scc))
	for _, sym := range scc {
		info := localFuncs[sym]
		if info == nil || info.Fn == nil {
			continue
		}
		newReturn := i.inferReturnForFunction(run, info, returnVectors, localFuncs)
		oldReturn := returnVectors[sym]
		merged := returnsummary.WidenForConvergence(oldReturn, newReturn)
		next[sym] = merged
		if !returnsummary.Equal(merged, oldReturn) {
			changed = true
		}
	}
	return next, changed
}

func applySCCIterationUpdates(
	returnVectors map[cfg.SymbolID][]typ.Type,
	scc []cfg.SymbolID,
	next map[cfg.SymbolID][]typ.Type,
) {
	for _, sym := range scc {
		if v, ok := next[sym]; ok {
			returnVectors[sym] = v
		}
	}
}
