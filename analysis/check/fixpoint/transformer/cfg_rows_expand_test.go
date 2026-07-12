package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestSolveAcyclicCFGExpandedRowsCrossProductsAtPoint(t *testing.T) {
	graph := cfg.New()
	middle := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), middle, false)
	graph.AddEdge(middle, graph.Exit(), false)
	reg := standard.Registry()
	arena := NewArena(reg)
	shape := Shape{Params: 1}
	param := arena.Root(Root{Kind: RootParam, Index: 0})
	result := symbol.ID(77)
	rows, err := SolveAcyclicCFGExpandedRows(graph, arena, SymbolicCFGRow{
		Guard: arena.True(), Values: map[symbol.ID]ValueTerm{1: param},
	}, func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
		if point != middle {
			return []SymbolicCFGRow{row}, nil
		}
		left, right := cloneCFGRow(row), cloneCFGRow(row)
		left.Values[result] = arena.Constant(typevalue.LiteralString(reg, "left"))
		right.Values[result] = arena.Constant(typevalue.LiteralString(reg, "right"))
		return []SymbolicCFGRow{left, right}, nil
	}, nil, SymbolicCFGOptions{Shape: shape, MaxRows: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rows[graph.Exit()]); got != 2 {
		t.Fatalf("expanded exit rows = %d, want two", got)
	}
}

func TestSolveAcyclicCFGExpandedRowsFailsAtomicallyAtExpansionBudget(t *testing.T) {
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	reg := standard.Registry()
	arena := NewArena(reg)
	shape := Shape{Params: 1}
	param := arena.Root(Root{Kind: RootParam, Index: 0})
	result := symbol.ID(88)
	rows, err := SolveAcyclicCFGExpandedRows(graph, arena, SymbolicCFGRow{
		Guard: arena.True(), Values: map[symbol.ID]ValueTerm{1: param},
	}, func(_ cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
		other := cloneCFGRow(row)
		other.Values[result] = arena.Constant(typevalue.LiteralString(reg, "distinct"))
		return []SymbolicCFGRow{row, other}, nil
	}, nil, SymbolicCFGOptions{Shape: shape, MaxRows: 1})
	if err == nil || !strings.Contains(err.Error(), "row budget") || rows != nil {
		t.Fatalf("expanded budget result = %#v/%v", rows, err)
	}
}
