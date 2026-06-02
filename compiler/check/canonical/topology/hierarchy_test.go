package topology

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestWalkHierarchyVisitsUniqueGraphsInBFSOrder(t *testing.T) {
	t.Parallel()

	grandchild := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	childA := &ast.FunctionExpr{
		Stmts: []ast.Stmt{&ast.LocalAssignStmt{
			Names: []string{"grandchild"},
			Exprs: []ast.Expr{grandchild},
		}},
	}
	childB := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	rootFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"a"}, Exprs: []ast.Expr{childA}},
			&ast.LocalAssignStmt{Names: []string{"b"}, Exprs: []ast.Expr{childB}},
		},
	}
	root := cfg.Build(rootFn)
	graphA := cfg.Build(childA)
	graphB := cfg.Build(childB)
	graphGrandchild := cfg.Build(grandchild)
	graphByFunc := map[*ast.FunctionExpr]*cfg.Graph{
		childA:     graphA,
		childB:     graphB,
		grandchild: graphGrandchild,
	}

	var order []uint64
	WalkHierarchy(HierarchyInput{
		Root: root,
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			return graphByFunc[fn]
		},
	}, func(node HierarchyNode) {
		order = append(order, node.Graph.ID())
	})

	want := []uint64{root.ID(), graphA.ID(), graphB.ID(), graphGrandchild.ID()}
	if len(order) != len(want) {
		t.Fatalf("visited graph count = %d (%v), want %d (%v)", len(order), order, len(want), want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("visit order = %v, want %v", order, want)
		}
	}
}

func TestWalkHierarchySkipsDuplicateGraphs(t *testing.T) {
	t.Parallel()

	childA := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	childB := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	rootFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"a"}, Exprs: []ast.Expr{childA}},
			&ast.LocalAssignStmt{Names: []string{"b"}, Exprs: []ast.Expr{childB}},
		},
	}
	root := cfg.Build(rootFn)
	sharedChild := cfg.Build(childA)

	var order []uint64
	WalkHierarchy(HierarchyInput{
		Root: root,
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			return sharedChild
		},
	}, func(node HierarchyNode) {
		order = append(order, node.Graph.ID())
	})

	want := []uint64{root.ID(), sharedChild.ID()}
	if len(order) != len(want) {
		t.Fatalf("visited graph count = %d (%v), want %d (%v)", len(order), order, len(want), want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("visit order = %v, want %v", order, want)
		}
	}
}

func TestWalkHierarchyWithStatePropagatesParentState(t *testing.T) {
	t.Parallel()

	grandchild := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	childA := &ast.FunctionExpr{
		Stmts: []ast.Stmt{&ast.LocalAssignStmt{
			Names: []string{"grandchild"},
			Exprs: []ast.Expr{grandchild},
		}},
	}
	childB := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	rootFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"a"}, Exprs: []ast.Expr{childA}},
			&ast.LocalAssignStmt{Names: []string{"b"}, Exprs: []ast.Expr{childB}},
		},
	}
	root := cfg.Build(rootFn)
	graphA := cfg.Build(childA)
	graphB := cfg.Build(childB)
	graphGrandchild := cfg.Build(grandchild)
	graphByFunc := map[*ast.FunctionExpr]*cfg.Graph{
		childA:     graphA,
		childB:     graphB,
		grandchild: graphGrandchild,
	}

	var order []uint64
	states := make(map[uint64]string)
	WalkHierarchyWithState(HierarchyStateInput[string]{
		Root:      root,
		RootState: "root",
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			return graphByFunc[fn]
		},
		ChildState: func(parent HierarchyStateNode[string], nested cfg.NestedFunc, _ *cfg.Graph) string {
			if nested.Func == childA {
				return parent.State + "/a"
			}
			if nested.Func == childB {
				return parent.State + "/b"
			}
			if nested.Func == grandchild {
				return parent.State + "/grandchild"
			}
			return parent.State
		},
	}, func(node HierarchyStateNode[string]) {
		order = append(order, node.Graph.ID())
		states[node.Graph.ID()] = node.State
	})

	wantOrder := []uint64{root.ID(), graphA.ID(), graphB.ID(), graphGrandchild.ID()}
	if len(order) != len(wantOrder) {
		t.Fatalf("visit order = %v, want %v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("visit order = %v, want %v", order, wantOrder)
		}
	}
	if states[root.ID()] != "root" ||
		states[graphA.ID()] != "root/a" ||
		states[graphB.ID()] != "root/b" ||
		states[graphGrandchild.ID()] != "root/a/grandchild" {
		t.Fatalf("states = %#v", states)
	}
}

func TestWalkHierarchyWithStateSkipsDuplicateGraphs(t *testing.T) {
	t.Parallel()

	childA := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	childB := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	rootFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"a"}, Exprs: []ast.Expr{childA}},
			&ast.LocalAssignStmt{Names: []string{"b"}, Exprs: []ast.Expr{childB}},
		},
	}
	root := cfg.Build(rootFn)
	sharedChild := cfg.Build(childA)

	var order []uint64
	WalkHierarchyWithState(HierarchyStateInput[int]{
		Root:      root,
		RootState: 1,
		GraphForFunc: func(*ast.FunctionExpr) *cfg.Graph {
			return sharedChild
		},
		ChildState: func(parent HierarchyStateNode[int], _ cfg.NestedFunc, _ *cfg.Graph) int {
			return parent.State + 1
		},
	}, func(node HierarchyStateNode[int]) {
		order = append(order, node.Graph.ID())
	})

	want := []uint64{root.ID(), sharedChild.ID()}
	if len(order) != len(want) {
		t.Fatalf("visited graph count = %d (%v), want %d (%v)", len(order), order, len(want), want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("visit order = %v, want %v", order, want)
		}
	}
}
