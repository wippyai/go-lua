package pipeline

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/internal"
)

// roundSettleMargin is the number of fixpoint rounds a chunk needs beyond its
// structural depth: return summaries, refinements and class snapshots settle
// over a few rounds once facts have crossed the chunk, and the last round
// observes no change.
const roundSettleMargin = 4

// roundBudget returns the number of fixpoint rounds the driver runs for the
// registered chunk: the configured round count, extended to the structural
// depth plus the settle margin when the chunk's structure needs more rounds.
func (d *Driver) roundBudget(store api.IterationStore) int {
	return max(d.cfg.MaxIterations, 1, structuralDepth(store)+roundSettleMargin)
}

// structuralDepth returns the propagation depth of the registered graph
// hierarchy: twice the maximum closure-nesting depth plus the maximum local
// call-chain depth.
//
// Each driver round carries interprocedural facts one closure level up (field
// writes, captured mutations), one closure level down (captured types) or one
// call hop. A fact produced in the deepest closure reaches the root and
// returns to the deepest closure of another branch in twice the nesting
// depth, and crosses the longest call chain in its depth.
func structuralDepth(store api.IterationStore) int {
	if store == nil {
		return 0
	}
	graphs := store.Graphs()
	return 2*nestingDepth(store, graphs) + callChainDepth(store, graphs)
}

// nestingDepth returns the maximum number of enclosing function graphs of any
// graph in graphs.
func nestingDepth(store api.IterationStore, graphs map[uint64]*cfg.Graph) int {
	depths := make(map[uint64]int, len(graphs))
	var depthOf func(id uint64) int
	depthOf = func(id uint64) int {
		if d, ok := depths[id]; ok {
			return d
		}
		depths[id] = 0
		meta, ok := store.NestedMetaFor(id)
		if !ok || meta.ParentGraphID == 0 || meta.ParentGraphID == id {
			return 0
		}
		d := depthOf(meta.ParentGraphID) + 1
		depths[id] = d
		return d
	}
	maxDepth := 0
	for id := range graphs {
		maxDepth = max(maxDepth, depthOf(id))
	}
	return maxDepth
}

// callChainDepth returns the number of edges on the longest path of the
// condensed local call graph: graphs are nodes and each call site whose
// callee resolves to a registered function graph is an edge. Mutually
// recursive functions form one node.
func callChainDepth(store api.IterationStore, graphs map[uint64]*cfg.Graph) int {
	moduleBindings := store.ModuleBindings()
	adj := make(map[uint64][]uint64, len(graphs))
	for id, graph := range graphs {
		seen := make(map[uint64]bool)
		callees := []uint64{}
		bindings := graph.Bindings()
		graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
			candidates := checkcallsite.CallableCalleeSymbolCandidates(info, graph, bindings, moduleBindings)
			for _, sym := range candidates {
				ref := store.FunctionRefBySym(sym)
				if ref == nil || ref.GraphID == 0 {
					continue
				}
				if _, known := graphs[ref.GraphID]; known && !seen[ref.GraphID] {
					seen[ref.GraphID] = true
					callees = append(callees, ref.GraphID)
				}
				break
			}
		})
		adj[id] = callees
	}

	sccs := internal.ComputeSCCs(adj)
	component := make(map[uint64]int, len(adj))
	for i, scc := range sccs {
		for _, id := range scc {
			component[id] = i
		}
	}
	// ComputeSCCs orders callees before callers, so every callee component's
	// depth is final when its caller component is visited.
	depth := make([]int, len(sccs))
	maxDepth := 0
	for i, scc := range sccs {
		for _, id := range scc {
			for _, callee := range adj[id] {
				if c := component[callee]; c != i {
					depth[i] = max(depth[i], depth[c]+1)
				}
			}
		}
		maxDepth = max(maxDepth, depth[i])
	}
	return maxDepth
}
