package transformer

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestSolveExactWTOCFGExpandedRowsZeroAndBodyBreakOnePlus(t *testing.T) {
	graph := cfg.New()
	head := graph.AddNode(cfg.NodeBranch)
	body := graph.AddNode(cfg.NodeBranch)
	latch := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), head, false)
	graph.AddEdge(head, body, true)
	graph.AddEdge(head, graph.Exit(), false)
	graph.AddEdge(body, graph.Exit(), true) // break after entering the body
	graph.AddEdge(body, latch, false)
	graph.AddEdge(latch, head, false)

	reg := standard.Registry()
	arena := NewArena(reg)
	local := symbol.ID(1)
	zero := arena.Constant(typevalue.LiteralInt(reg, 0))
	one := arena.Constant(typevalue.LiteralInt(reg, 1))
	rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena,
		SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{local: zero}},
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			if point == body {
				row.Values[local] = one
			}
			return []SymbolicCFGRow{row}, nil
		}, exactWTOAllEdges(arena), SymbolicExactWTOOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertExactWTOValues(t, rows[graph.Exit()], local, zero, one)
}

func TestSolveExactWTOCFGExpandedRowsStabilizesAfterTwoLaps(t *testing.T) {
	graph, head, body := exactWTOSimpleLoop()
	reg := standard.Registry()
	arena := NewArena(reg)
	local := symbol.ID(2)
	values := []ValueTerm{
		arena.Constant(typevalue.LiteralInt(reg, 0)),
		arena.Constant(typevalue.LiteralInt(reg, 1)),
		arena.Constant(typevalue.LiteralInt(reg, 2)),
	}
	rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena,
		SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{local: values[0]}},
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			if point == body {
				switch row.Values[local] {
				case values[0]:
					row.Values[local] = values[1]
				case values[1]:
					row.Values[local] = values[2]
				}
			}
			return []SymbolicCFGRow{row}, nil
		}, exactWTOAllEdges(arena), SymbolicExactWTOOptions{MaxIterations: 8})
	if err != nil {
		t.Fatal(err)
	}
	_ = head
	assertExactWTOValues(t, rows[graph.Exit()], local, values...)
}

func TestSolveExactWTOCFGExpandedRowsExpandsWholeRowsInCycle(t *testing.T) {
	graph, _, body := exactWTOSimpleLoop()
	reg := standard.Registry()
	arena := NewArena(reg)
	left, right := symbol.ID(3), symbol.ID(4)
	zero := arena.Constant(typevalue.LiteralInt(reg, 0))
	one := arena.Constant(typevalue.LiteralInt(reg, 1))
	rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena,
		SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}},
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			if point != body {
				return []SymbolicCFGRow{row}, nil
			}
			a, b := cloneCFGRow(row), cloneCFGRow(row)
			a.Values[left], a.Values[right] = zero, one
			b.Values[left], b.Values[right] = one, zero
			return []SymbolicCFGRow{a, b}, nil
		}, exactWTOAllEdges(arena), SymbolicExactWTOOptions{})
	if err != nil {
		t.Fatal(err)
	}
	exit := rows[graph.Exit()]
	if len(exit) != 3 { // zero-iteration plus two correlated 1+ alternatives
		t.Fatalf("expanded loop exits = %d, want three", len(exit))
	}
	for _, row := range exit {
		if len(row.Values) != 0 && row.Values[left] == row.Values[right] {
			t.Fatalf("expanded correlation was lost: %#v", row.Values)
		}
	}
}

func TestSolveExactWTOCFGExpandedRowsNestedReentryAndSiblingTransition(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	initial := SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}}

	nested := cfg.New()
	outer := nested.AddNode(cfg.NodeBranch)
	inner := nested.AddNode(cfg.NodeBranch)
	innerBody := nested.AddNode(cfg.NodeAssign)
	latch := nested.AddNode(cfg.NodeAssign)
	nested.AddEdge(nested.Entry(), outer, false)
	nested.AddEdge(outer, inner, true)
	nested.AddEdge(outer, nested.Exit(), false)
	nested.AddEdge(inner, innerBody, true)
	nested.AddEdge(inner, latch, false)
	nested.AddEdge(innerBody, inner, false)
	nested.AddEdge(latch, outer, false)
	if rows, err := SolveExactWTOCFGExpandedRows(context.Background(), nested, arena, initial,
		exactWTOIdentityExpand, exactWTOAllEdges(arena), SymbolicExactWTOOptions{}); err != nil || len(rows[nested.Exit()]) != 1 {
		t.Fatalf("nested closure = %d rows, %v", len(rows[nested.Exit()]), err)
	}

	sibling := cfg.New()
	left := sibling.AddNode(cfg.NodeBranch)
	right := sibling.AddNode(cfg.NodeBranch)
	leftBody := sibling.AddNode(cfg.NodeAssign)
	rightBody := sibling.AddNode(cfg.NodeAssign)
	sibling.AddEdge(sibling.Entry(), left, false)
	sibling.AddEdge(left, leftBody, true)
	sibling.AddEdge(left, right, false)
	sibling.AddEdge(leftBody, left, false)
	sibling.AddEdge(right, rightBody, true)
	sibling.AddEdge(right, sibling.Exit(), false)
	sibling.AddEdge(rightBody, right, false)
	if rows, err := SolveExactWTOCFGExpandedRows(context.Background(), sibling, arena, initial,
		exactWTOIdentityExpand, exactWTOAllEdges(arena), SymbolicExactWTOOptions{}); err != nil || len(rows[sibling.Exit()]) != 1 {
		t.Fatalf("sibling closure = %d rows, %v", len(rows[sibling.Exit()]), err)
	}
}

func BenchmarkSolveExactWTOCFGExpandedRowsOwnedCopies(b *testing.B) {
	graph, _, _ := exactWTOSimpleLoop()
	reg := standard.Registry()
	arena := NewArena(reg)
	values := make(map[symbol.ID]ValueTerm, 16)
	for i := 0; i < 16; i++ {
		values[symbol.ID(i+1)] = arena.Constant(typevalue.LiteralInt(reg, int64(i)))
	}
	initial := SymbolicCFGRow{
		Guard:      arena.True(),
		Values:     values,
		Operations: make([]Operation, 8),
		Output:     summaryForCloneBenchmark(8),
	}
	branch := exactWTOAllEdges(arena)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena, initial, exactWTOIdentityExpand, branch, SymbolicExactWTOOptions{})
		if err != nil || len(rows[graph.Exit()]) != 1 {
			b.Fatalf("rows/error = %d/%v", len(rows[graph.Exit()]), err)
		}
	}
}

func TestSolveExactWTOCFGExpandedRowsPreservesCallerOwnedInitialRow(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	local := symbol.ID(91)
	initial := SymbolicCFGRow{
		Guard:      arena.True(),
		Values:     map[symbol.ID]ValueTerm{local: arena.Constant(typevalue.LiteralInt(reg, 1))},
		Operations: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn}},
		Output:     summaryForCloneBenchmark(1),
	}
	want := cloneCFGRow(initial)
	mutate := func(_ cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
		row.Values[local] = arena.Constant(typevalue.LiteralInt(reg, 2))
		row.Operations[0].Slot = 1
		row.Output.Returns[0] = product.Bottom(reg)
		return []SymbolicCFGRow{row}, nil
	}
	graphs := map[string]cfg.Graph{}
	linear := cfg.New()
	point := linear.AddNode(cfg.NodeAssign)
	linear.AddEdge(linear.Entry(), point, false)
	linear.AddEdge(point, linear.Exit(), false)
	graphs["transient DAG"] = linear
	loop, _, _ := exactWTOSimpleLoop()
	graphs["retained cycle"] = loop
	for name, graph := range graphs {
		t.Run(name, func(t *testing.T) {
			if _, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena, initial, mutate, exactWTOAllEdges(arena), SymbolicExactWTOOptions{}); err != nil {
				t.Fatal(err)
			}
			if !equalCFGRow(arena, initial, want) {
				t.Fatal("solver mutated caller-owned initial row")
			}
		})
	}
}

func summaryForCloneBenchmark(width int) summary.Summary {
	values := make([]product.Value, width)
	for i := range values {
		values[i] = product.Top()
	}
	return summary.Summary{Returns: values, NormalReturnParams: append([]product.Value(nil), values...)}
}

func TestSolveExactWTOCFGExpandedRowsRejectsUncertifiedComponentHead(t *testing.T) {
	graph := cfg.New()
	head := graph.AddNode(cfg.NodeBranch)
	graph.AddEdge(graph.Entry(), head, false)
	graph.AddEdge(head, head, true)
	graph.AddEdge(head, graph.Exit(), false)
	reg := standard.Registry()
	arena := NewArena(reg)
	rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena,
		SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}},
		exactWTOIdentityExpand, exactWTOAllEdges(arena), SymbolicExactWTOOptions{})
	if err == nil || rows != nil || !strings.Contains(err.Error(), "body and") {
		t.Fatalf("uncertified head = %#v, %v", rows, err)
	}
}

func TestSolveExactWTOCFGExpandedRowsPreservesCorrelationUnderHashCollision(t *testing.T) {
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	truePath := graph.AddNode(cfg.NodeAssign)
	falsePath := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, truePath, true)
	graph.AddEdge(branch, falsePath, false)
	graph.AddEdge(truePath, graph.Exit(), false)
	graph.AddEdge(falsePath, graph.Exit(), false)
	reg := standard.Registry()
	arena := NewArena(reg)
	left, right := symbol.ID(10), symbol.ID(11)
	zero := arena.Constant(typevalue.LiteralInt(reg, 0))
	one := arena.Constant(typevalue.LiteralInt(reg, 1))
	rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena,
		SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}},
		exactWTOIdentityExpand,
		func(_ cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
			if cond {
				row.Values[left], row.Values[right] = zero, one
			} else {
				row.Values[left], row.Values[right] = one, zero
			}
			return row, arena.True(), nil
		}, SymbolicExactWTOOptions{rowHash: func(SymbolicCFGRow) uint64 { return 7 }})
	if err != nil {
		t.Fatal(err)
	}
	exit := rows[graph.Exit()]
	if len(exit) != 2 {
		t.Fatalf("collision rows = %d, want two exact alternatives", len(exit))
	}
	for _, row := range exit {
		if row.Values[left] == row.Values[right] {
			t.Fatalf("correlation was column-merged: %#v", row.Values)
		}
	}
}

func TestSolveExactWTOCFGExpandedRowsCancellationAndEffectsAreAtomic(t *testing.T) {
	graph, _, body := exactWTOSimpleLoop()
	reg := standard.Registry()
	arena := NewArena(reg)
	initial := SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}}

	for name, run := range map[string]func() (map[cfg.Point][]SymbolicCFGRow, error){
		"iterations": func() (map[cfg.Point][]SymbolicCFGRow, error) {
			return SolveExactWTOCFGExpandedRows(context.Background(), graph, arena, initial,
				exactWTOIdentityExpand, exactWTOAllEdges(arena), SymbolicExactWTOOptions{MaxIterations: 1})
		},
		"effects": func() (map[cfg.Point][]SymbolicCFGRow, error) {
			return SolveExactWTOCFGExpandedRows(context.Background(), graph, arena, initial,
				func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
					if point == body {
						row.Effects = []EffectTerm{1}
					}
					return []SymbolicCFGRow{row}, nil
				}, exactWTOAllEdges(arena), SymbolicExactWTOOptions{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			rows, err := run()
			if err == nil || rows != nil {
				t.Fatalf("atomic failure = %#v, %v", rows, err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	rows, err := SolveExactWTOCFGExpandedRows(ctx, graph, arena, initial,
		func(_ cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			cancel()
			return []SymbolicCFGRow{row}, nil
		}, exactWTOAllEdges(arena), SymbolicExactWTOOptions{})
	if err == nil || !strings.Contains(err.Error(), "canceled") || rows != nil {
		t.Fatalf("cancellation = %#v, %v", rows, err)
	}
}

func TestSolveExactWTOCFGExpandedRowsRetainsMoreThanOldRowLimit(t *testing.T) {
	graph, _, _ := exactWTOSimpleLoop()
	reg := standard.Registry()
	arena := NewArena(reg)
	local := symbol.ID(9101)
	initial := SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}}
	rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena, initial,
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			if point != graph.Entry() {
				return []SymbolicCFGRow{row}, nil
			}
			out := make([]SymbolicCFGRow, 513)
			for index := range out {
				out[index] = cloneCFGRow(row)
				out[index].Values[local] = arena.Constant(typevalue.LiteralInt(reg, int64(index)))
			}
			return out, nil
		}, exactWTOAllEdges(arena), SymbolicExactWTOOptions{SymbolicCFGOptions: SymbolicCFGOptions{MaxRows: 1}})
	if err != nil {
		t.Fatal(err)
	}
	exit := rows[graph.Exit()]
	if len(exit) != 513 {
		t.Fatalf("exact WTO retained %d alternatives, want 513", len(exit))
	}
}

func TestSolveExactWTOCFGExpandedRowsUnreachableDeterministic(t *testing.T) {
	graph, _, _ := exactWTOSimpleLoop()
	dead := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(dead, dead, false)
	reg := standard.Registry()
	arena := NewArena(reg)
	initial := SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}}
	var want string
	for i := 0; i < 50; i++ {
		rows, err := SolveExactWTOCFGExpandedRows(context.Background(), graph, arena, initial,
			exactWTOIdentityExpand, exactWTOAllEdges(arena), SymbolicExactWTOOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := rows[dead]; ok {
			t.Fatal("unreachable row was published")
		}
		got := fmt.Sprint(rows)
		if i == 0 {
			want = got
		} else if got != want {
			t.Fatalf("run %d differs:\nwant %s\ngot  %s", i, want, got)
		}
	}
	if reflect.ValueOf(want).Len() == 0 {
		t.Fatal("empty deterministic rendering")
	}
}

func exactWTOSimpleLoop() (*cfg.CFG, cfg.Point, cfg.Point) {
	graph := cfg.New()
	head := graph.AddNode(cfg.NodeBranch)
	body := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), head, false)
	graph.AddEdge(head, body, true)
	graph.AddEdge(head, graph.Exit(), false)
	graph.AddEdge(body, head, false)
	return graph, head, body
}

func exactWTOIdentityExpand(_ cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
	return []SymbolicCFGRow{row}, nil
}

func exactWTOAllEdges(arena *Arena) SymbolicCFGBranch {
	return func(_ cfg.Point, row SymbolicCFGRow, _ bool) (SymbolicCFGRow, Guard, error) {
		return row, arena.True(), nil
	}
}

func assertExactWTOValues(t *testing.T, rows []SymbolicCFGRow, local symbol.ID, want ...ValueTerm) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d (%#v)", len(rows), len(want), rows)
	}
	seen := make(map[ValueTerm]bool, len(rows))
	for _, row := range rows {
		seen[row.Values[local]] = true
	}
	for _, value := range want {
		if !seen[value] {
			t.Fatalf("missing value term %d in %#v", value, seen)
		}
	}
}
