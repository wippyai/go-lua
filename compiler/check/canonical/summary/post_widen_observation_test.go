package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestSelectPostWidenObservationRefs_MethodReceiverEffectsKeepDiscoveryOrder(t *testing.T) {
	root := summary.FuncRef{GraphID: 1}
	methodA := summary.FuncRef{GraphID: 2}
	methodB := summary.FuncRef{GraphID: 3}
	plain := summary.FuncRef{GraphID: 4}
	refs := []summary.FuncRef{root, methodA, plain, methodB}
	receiverSummary := summary.Summary{
		ReceiverEffects: flow.ReceiverMutations(0, []flow.ReceiverMutation{{
			Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "last_node_id"}},
		}}),
	}

	got := summary.SelectPostWidenObservationRefs(summary.PostWidenObservationInput{
		Refs: refs,
		Root: root,
		Summary: func(ref summary.FuncRef) summary.Summary {
			switch ref {
			case methodA, methodB, plain:
				return receiverSummary
			default:
				return summary.Summary{}
			}
		},
		IsMethod: func(ref summary.FuncRef) bool {
			return ref == methodA || ref == methodB
		},
	})

	want := []summary.FuncRef{methodA, methodB}
	assertRefs(t, got, want)
}

func TestSelectPostWidenObservationRefs_ReturnedCapturedCellObservesChildBeforeParent(t *testing.T) {
	parentFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"obj"},
				Exprs: []ast.Expr{&ast.TableExpr{}},
			},
			&ast.LocalAssignStmt{
				Names: []string{"install"},
				Exprs: []ast.Expr{&ast.FunctionExpr{
					Stmts: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{&ast.AttrGetExpr{
								Object: &ast.IdentExpr{Value: "obj"},
								Key:    &ast.StringExpr{Value: "get_value"},
							}},
							Rhs: []ast.Expr{&ast.FunctionExpr{}},
						},
					},
				}},
			},
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "obj"}}},
		},
	}
	parentGraph := cfg.Build(parentFn)
	if parentGraph == nil || parentGraph.Bindings() == nil {
		t.Fatal("parent graph missing bindings")
	}
	nested := parentGraph.NestedFunctions()
	if len(nested) != 1 {
		t.Fatalf("nested function count = %d, want 1", len(nested))
	}
	childGraph := cfg.BuildWithBindings(nested[0].Func, parentGraph.Bindings())
	root := summary.FuncRef{GraphID: 1}
	parent := summary.FuncRef{GraphID: 2}
	child := summary.FuncRef{GraphID: 3}
	graphs := map[summary.FuncRef]*cfg.Graph{
		parent: parentGraph,
		child:  childGraph,
	}

	got := summary.SelectPostWidenObservationRefs(summary.PostWidenObservationInput{
		Refs: []summary.FuncRef{root, parent, child},
		Root: root,
		Summary: func(summary.FuncRef) summary.Summary {
			return summary.Summary{}
		},
		Graph: func(ref summary.FuncRef) *cfg.Graph {
			return graphs[ref]
		},
		Nested: func(ref summary.FuncRef) []summary.FuncRef {
			if ref == parent {
				return []summary.FuncRef{child}
			}
			return nil
		},
		Parent: func(ref summary.FuncRef) (summary.FuncRef, bool) {
			if ref == child {
				return parent, true
			}
			return summary.FuncRef{}, false
		},
	})

	want := []summary.FuncRef{child, parent}
	assertRefs(t, got, want)
}

func assertRefs(t *testing.T, got, want []summary.FuncRef) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("refs len = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("refs[%d] = %v, want %v; all=%v", i, got[i], want[i], got)
		}
	}
}
