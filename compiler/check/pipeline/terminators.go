package pipeline

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// structuralTerminators returns the functions of the module that never return
// normally: every path from their entry ends in a call that never returns, to
// a builtin such as error() or to another such function. It is the least
// fixpoint over the module's function graphs, computed from control flow and
// callee effects alone, so the first fixpoint iteration already knows that a
// call to such a function ends its path.
func structuralTerminators(store api.IterationStore, globalTypes map[string]typ.Type) map[cfg.SymbolID]*constraint.FunctionRefinement {
	if store == nil {
		return nil
	}
	refs := store.FunctionRefs()
	if len(refs) == 0 {
		return nil
	}
	graphs := store.Graphs()
	moduleBindings := store.ModuleBindings()

	found := make(map[cfg.SymbolID]*constraint.FunctionRefinement)
	known := terminatorStore(found)
	lookup := func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		return effects.LookupRefinementBySym(known, moduleBindings, globalTypes, sym)
	}

	for changed := true; changed; {
		changed = false
		for _, ref := range refs {
			if ref == nil || found[ref.Sym] != nil {
				continue
			}
			graph := graphs[ref.GraphID]
			if graph == nil || !pathsEndInTerminatingCalls(graph, lookup, moduleBindings) {
				continue
			}
			found[ref.Sym] = &constraint.FunctionRefinement{Terminates: true}
			changed = true
		}
	}
	return found
}

// pathsEndInTerminatingCalls reports whether no return and not the exit of
// graph is reachable from its entry when control stops at every point with a
// call that never returns.
func pathsEndInTerminatingCalls(graph *cfg.Graph, lookup constraint.RefinementLookupBySym, moduleBindings *bind.BindingTable) bool {
	flowGraph := graph.CFG()
	if flowGraph == nil {
		return false
	}
	exit := flowGraph.Exit()
	visited := make(map[cfg.Point]bool)
	work := []cfg.Point{flowGraph.Entry()}
	for len(work) > 0 {
		p := work[len(work)-1]
		work = work[:len(work)-1]
		if visited[p] {
			continue
		}
		visited[p] = true
		if p == exit {
			return false
		}
		if node := flowGraph.Node(p); node != nil && node.Kind == cfg.NodeReturn {
			return false
		}
		if cond.PointHasTerminatingCallSite(graph, p, nil, nil, lookup, moduleBindings) {
			continue
		}
		work = append(work, flowGraph.Successors(p)...)
	}
	return true
}

// terminatorStore serves the terminators found so far as refinements.
type terminatorStore map[cfg.SymbolID]*constraint.FunctionRefinement

func (s terminatorStore) LookupRefinementBySym(sym cfg.SymbolID) *constraint.FunctionRefinement {
	return s[sym]
}
