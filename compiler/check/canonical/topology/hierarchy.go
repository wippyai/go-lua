package topology

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// HierarchyInput is the canonical CFG hierarchy traversal input.
type HierarchyInput struct {
	Root         *cfg.Graph
	GraphForFunc func(*ast.FunctionExpr) *cfg.Graph
}

// HierarchyNode is one graph reached by deterministic breadth-first traversal.
type HierarchyNode struct {
	Graph  *cfg.Graph
	Parent *cfg.Graph
	Nested cfg.NestedFunc
}

// HierarchyStateInput is a deterministic hierarchy traversal with caller-owned
// state carried from parent graph to child graph.
type HierarchyStateInput[S any] struct {
	Root         *cfg.Graph
	RootState    S
	GraphForFunc func(*ast.FunctionExpr) *cfg.Graph

	// ChildState derives the state for child from the already-visited parent.
	// When nil, children inherit the parent state unchanged.
	ChildState func(parent HierarchyStateNode[S], nested cfg.NestedFunc, child *cfg.Graph) S
}

// HierarchyStateNode is one graph reached by deterministic breadth-first
// traversal, paired with the state computed for that graph.
type HierarchyStateNode[S any] struct {
	Graph  *cfg.Graph
	Parent *cfg.Graph
	Nested cfg.NestedFunc
	State  S
}

// WalkHierarchy visits each graph in the transitive nested-function hierarchy
// exactly once, in deterministic BFS order: root, then nested functions in each
// parent's CFG discovery order.
func WalkHierarchy(in HierarchyInput, visit func(HierarchyNode)) {
	if in.Root == nil || visit == nil {
		return
	}
	queue := []HierarchyNode{{Graph: in.Root}}
	enqueued := map[uint64]bool{in.Root.ID(): true}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visit(node)
		if in.GraphForFunc == nil || node.Graph == nil {
			continue
		}
		for _, nested := range node.Graph.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			ng := in.GraphForFunc(nested.Func)
			if ng == nil || enqueued[ng.ID()] {
				continue
			}
			enqueued[ng.ID()] = true
			queue = append(queue, HierarchyNode{
				Graph:  ng,
				Parent: node.Graph,
				Nested: nested,
			})
		}
	}
}

// WalkHierarchyWithState visits each graph in deterministic BFS order while
// carrying caller-owned state across lexical parent edges.
func WalkHierarchyWithState[S any](in HierarchyStateInput[S], visit func(HierarchyStateNode[S])) {
	if in.Root == nil || visit == nil {
		return
	}
	queue := []HierarchyStateNode[S]{{Graph: in.Root, State: in.RootState}}
	enqueued := map[uint64]bool{in.Root.ID(): true}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visit(node)
		if in.GraphForFunc == nil || node.Graph == nil {
			continue
		}
		for _, nested := range node.Graph.NestedFunctions() {
			if nested.Func == nil {
				continue
			}
			ng := in.GraphForFunc(nested.Func)
			if ng == nil || enqueued[ng.ID()] {
				continue
			}
			enqueued[ng.ID()] = true
			childState := node.State
			if in.ChildState != nil {
				childState = in.ChildState(node, nested, ng)
			}
			queue = append(queue, HierarchyStateNode[S]{
				Graph:  ng,
				Parent: node.Graph,
				Nested: nested,
				State:  childState,
			})
		}
	}
}
