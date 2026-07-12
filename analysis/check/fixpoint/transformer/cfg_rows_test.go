package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

const cfgRowResult symbol.ID = 101

func TestSolveAcyclicCFGRowsPreservesDiamondReturnCorrelation(t *testing.T) {
	graph, branchPoint, left, right, join := testDiamondCFG()
	reg := standard.Registry()
	shape := Shape{Params: 1}
	builder, certificate := emptyBuilder(t, reg, shape, nil)
	arena := builder.Arena()
	param := arena.Root(Root{Kind: RootParam, Index: 0})
	wantTruthy := typevalue.LiteralString(reg, "truthy-result")
	wantFalsy := typevalue.LiteralString(reg, "falsy-result")

	rows, err := SolveAcyclicCFGRows(
		graph,
		arena,
		SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{1: param}},
		func(point cfg.Point, row SymbolicCFGRow) (SymbolicCFGRow, error) {
			switch point {
			case left:
				row.Values[cfgRowResult] = arena.Constant(wantTruthy)
			case right:
				row.Values[cfgRowResult] = arena.Constant(wantFalsy)
			}
			return row, nil
		},
		func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
			if point != branchPoint {
				t.Fatalf("branch callback point = %d, want %d", point, branchPoint)
			}
			if cond {
				return row, arena.Truthy(row.Values[1]), nil
			}
			return row, arena.Falsy(row.Values[1]), nil
		},
		SymbolicCFGOptions{Shape: shape},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rows[join]); got != 2 {
		t.Fatalf("join rows = %d, want 2", got)
	}
	if got := len(rows[graph.Exit()]); got != 2 {
		t.Fatalf("exit rows = %d, want 2", got)
	}

	relationRows := make([]Row, len(rows[graph.Exit()]))
	for i, row := range rows[graph.Exit()] {
		relationRows[i] = Row{Guard: row.Guard, Ops: []Operation{{
			Kind: OutputReturn, Descriptor: DescriptorReturn, Value: row.Values[cfgRowResult],
		}}}
	}
	relation, err := builder.Build(certificate, relationRows)
	if err != nil {
		t.Fatal(err)
	}
	assertCFGRelationSpecialization(t, relation,
		typevalue.LiteralBool(reg, true), wantTruthy)
	assertCFGRelationSpecialization(t, relation,
		typevalue.LiteralBool(reg, false), wantFalsy)
}

func TestSolveAcyclicCFGRowsFailsAtomicallyAtRowBudget(t *testing.T) {
	graph, branchPoint, left, right, _ := testDiamondCFG()
	reg := standard.Registry()
	arena := NewArena(reg)
	shape := Shape{Params: 1}
	param := arena.Root(Root{Kind: RootParam, Index: 0})

	rows, err := SolveAcyclicCFGRows(
		graph,
		arena,
		SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{1: param}},
		func(point cfg.Point, row SymbolicCFGRow) (SymbolicCFGRow, error) {
			if point == left || point == right {
				row.Values[cfgRowResult] = arena.Constant(typevalue.LiteralInt(reg, int64(point)))
			}
			return row, nil
		},
		func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
			if point != branchPoint {
				t.Fatalf("branch callback point = %d, want %d", point, branchPoint)
			}
			if cond {
				return row, arena.Truthy(row.Values[1]), nil
			}
			return row, arena.Falsy(row.Values[1]), nil
		},
		SymbolicCFGOptions{Shape: shape, MaxRows: 1},
	)
	if err == nil || !strings.Contains(err.Error(), "symbolic CFG row budget") {
		t.Fatalf("error = %v, want row budget failure", err)
	}
	if rows != nil {
		t.Fatalf("rows = %#v, want atomic nil result", rows)
	}
}

func TestSolveAcyclicCFGRowsRejectsCyclesAndWrongBoundaryShape(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	param := arena.Root(Root{Kind: RootParam, Index: 0})

	linear := cfg.New()
	linear.AddEdge(linear.Entry(), linear.Exit(), false)
	if rows, err := SolveAcyclicCFGRows(linear, arena,
		SymbolicCFGRow{Guard: arena.Truthy(param)}, nil, nil, SymbolicCFGOptions{}); err == nil || rows != nil || !strings.Contains(err.Error(), "invalid for boundary shape") {
		t.Fatalf("wrong-shape result = %#v, %v", rows, err)
	}

	cyclic := cfg.New()
	loop := cyclic.AddNode(cfg.NodeBranch)
	cyclic.AddEdge(cyclic.Entry(), loop, false)
	cyclic.AddEdge(loop, loop, true)
	cyclic.AddEdge(loop, cyclic.Exit(), false)
	if rows, err := SolveAcyclicCFGRows(cyclic, arena,
		SymbolicCFGRow{Guard: arena.True()}, nil, nil, SymbolicCFGOptions{}); err == nil || rows != nil || err.Error() != "transformer: cyclic CFG requires WTO/SCC rows" {
		t.Fatalf("cyclic result = %#v, %v", rows, err)
	}
}

func assertCFGRelationSpecialization(t *testing.T, relation Relation, input, want product.Value) {
	t.Helper()
	cursor, err := NewBindingCursor(relation.Shape(), []product.Value{input}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, exact := relation.Specialize(cursor, nil, nil)
	if !exact {
		t.Fatal("diamond relation did not specialize exactly")
	}
	if len(got.Returns) != 1 || !product.Equal(relation.arena.reg, got.Returns[0], want) {
		t.Fatalf("specialized summary = %#v, want %#v", got, summary.Summary{Returns: []product.Value{want}})
	}
}

func testDiamondCFG() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point) {
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	left := graph.AddNode(cfg.NodeAssign)
	right := graph.AddNode(cfg.NodeAssign)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, left, true)
	graph.AddEdge(branch, right, false)
	graph.AddEdge(left, join, false)
	graph.AddEdge(right, join, false)
	graph.AddEdge(join, graph.Exit(), false)
	return graph, branch, left, right, join
}
