package transformer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// TestExactDenseDAGAcyclicDifferential freezes the zero-component WTO case
// against the retired production executor. It compares every reachable point
// extensionally, then compares canonical Relation order and digest at the only
// boundary consumed by PreparedPlanCompiler. Intermediate discovery order is
// intentionally executor-private; Builder canonicalizes the published order.
func TestExactDenseDAGAcyclicDifferential(t *testing.T) {
	const result symbol.ID = 701
	reg := standard.Registry()

	tests := []struct {
		name   string
		graph  func() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point)
		expand bool
	}{
		{
			name: "linear",
			graph: func() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point) {
				graph := cfg.New()
				point := graph.AddNode(cfg.NodeAssign)
				graph.AddEdge(graph.Entry(), point, false)
				graph.AddEdge(point, graph.Exit(), false)
				return graph, 0, point, 0
			},
		},
		{
			name: "diamond_branch_correlation",
			graph: func() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point) {
				graph, branch, left, right, _ := testDiamondCFG()
				return graph, branch, left, right
			},
		},
		{
			name: "expanded_direct_rows",
			graph: func() (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point) {
				graph := cfg.New()
				point := graph.AddNode(cfg.NodeCall)
				graph.AddEdge(graph.Entry(), point, false)
				graph.AddEdge(point, graph.Exit(), false)
				return graph, 0, point, 0
			},
			expand: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, branchPoint, left, right := test.graph()
			shape := Shape{Params: 1}
			builder, certificate := emptyBuilder(t, reg, shape, nil)
			arena := builder.Arena()
			param := arena.Root(Root{Kind: RootParam, Index: 0})
			initial := SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{1: param}}
			transfer := func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
				switch {
				case test.expand && point == left:
					first, second := cloneCFGRow(row), cloneCFGRow(row)
					first.Values[result] = arena.Constant(typevalue.LiteralString(reg, "first"))
					second.Values[result] = arena.Constant(typevalue.LiteralString(reg, "second"))
					return []SymbolicCFGRow{first, second}, nil
				case branchPoint != 0 && point == left:
					row.Values[result] = arena.Constant(typevalue.LiteralString(reg, "left"))
				case branchPoint != 0 && point == right:
					row.Values[result] = arena.Constant(typevalue.LiteralString(reg, "right"))
				case branchPoint == 0 && point == left:
					row.Values[result] = arena.Constant(typevalue.LiteralString(reg, "linear"))
				}
				return []SymbolicCFGRow{row}, nil
			}
			branch := func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
				if point != branchPoint {
					return SymbolicCFGRow{}, 0, fmt.Errorf("unexpected branch %d", point)
				}
				if cond {
					return row, arena.Truthy(param), nil
				}
				return row, arena.Falsy(param), nil
			}
			options := SymbolicCFGOptions{Shape: shape}
			legacy, err := SolveAcyclicCFGExpandedRows(graph, arena, initial, transfer, branch, options)
			if err != nil {
				t.Fatal(err)
			}
			tape, err := compileSymbolicWTOTape(graph)
			if err != nil {
				t.Fatal(err)
			}
			if len(tape.components) != 0 {
				t.Fatalf("DAG tape components = %d, want zero", len(tape.components))
			}
			dense, err := solveExactWTOCFGExpandedRowsWithTape(context.Background(), graph, tape, arena, initial, transfer, branch,
				SymbolicExactWTOOptions{SymbolicCFGOptions: options})
			if err != nil {
				t.Fatal(err)
			}
			assertCFGRowMapsEqual(t, arena, graph, legacy, dense)

			legacyRelation := buildDifferentialExitRelation(t, builder, certificate, legacy[graph.Exit()], result)
			denseRelation := buildDifferentialExitRelation(t, builder, certificate, dense[graph.Exit()], result)
			if !EqualRelation(legacyRelation, denseRelation) {
				t.Fatal("dense DAG relation differs from acyclic relation")
			}
			legacyDigest := canonicalRelationTestDigest(legacyRelation)
			if denseDigest := canonicalRelationTestDigest(denseRelation); denseDigest != legacyDigest {
				t.Fatalf("dense relation digest = %x, want %x", denseDigest, legacyDigest)
			}
		})
	}
}

func assertCFGRowMapsEqual(t *testing.T, arena *Arena, graph cfg.Graph, left, right map[cfg.Point][]SymbolicCFGRow) {
	t.Helper()
	for _, point := range cfg.RPOReadOnly(graph) {
		leftRows, rightRows := left[point], right[point]
		if len(leftRows) != len(rightRows) {
			t.Fatalf("point %d row count = %d, want %d", point, len(rightRows), len(leftRows))
		}
		for i := range leftRows {
			found := false
			for j := range rightRows {
				if equalCFGRow(arena, leftRows[i], rightRows[j]) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("point %d legacy row %d is missing from dense result", point, i)
			}
		}
	}
}

func buildDifferentialExitRelation(t *testing.T, builder *Builder, certificate SemanticCertificate, rows []SymbolicCFGRow, result symbol.ID) Relation {
	t.Helper()
	relationRows := make([]Row, len(rows))
	for i, row := range rows {
		relationRows[i] = Row{Guard: row.Guard}
		if value := row.Values[result]; value != 0 {
			relationRows[i].Ops = []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: value}}
		}
	}
	relation, err := builder.Build(certificate, relationRows)
	if err != nil {
		t.Fatal(err)
	}
	return relation
}

func canonicalRelationTestDigest(relation Relation) [32]byte {
	text := fmt.Sprintf("%d/%d/%t/%s\n", relation.shape.Params, relation.shape.Captures, relation.widened, relation.contextual)
	for _, row := range relation.rows {
		text += rowKey(relation.arena, relation.effects, row) + "\n"
	}
	return sha256.Sum256([]byte(text))
}

// evaluatePreparedAcyclicLegacy is a differential oracle only. Production has
// one executor; this copy of its former DAG seam is intentionally confined to
// tests until the compatibility window closes.
func evaluatePreparedAcyclicLegacy(t *testing.T, prepared *PreparedPlanCompiler, view RelationView, direct *DirectCallCatalog) Relation {
	t.Helper()
	relation, err := evaluatePreparedAcyclicLegacyRaw(prepared, view, direct)
	if err != nil {
		t.Fatal(err)
	}
	return relation
}

func evaluatePreparedAcyclicLegacyRaw(prepared *PreparedPlanCompiler, view RelationView, direct *DirectCallCatalog) (Relation, error) {
	base := prepared.base
	base.directCalls = direct
	initial := SymbolicCFGRow{
		Guard: prepared.builder.Arena().True(), Values: base.locals, genericBindings: base.genericBindings,
	}
	if prepared.shape.Params != 0 {
		initial.Output.NormalReturnParams = make([]product.Value, prepared.shape.Params)
		for i := range initial.Output.NormalReturnParams {
			initial.Output.NormalReturnParams[i] = product.Top()
		}
	}
	rowsByPoint, err := SolveAcyclicCFGExpandedRows(prepared.graph, prepared.builder.Arena(), initial,
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			return prepared.lowerPreparedPoint(base, view, direct, point, row)
		},
		func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
			return compileBranchEdge(base, point, row, cond)
		},
		SymbolicCFGOptions{Shape: prepared.shape},
	)
	if err != nil {
		return Relation{}, err
	}
	exit := rowsByPoint[prepared.graph.Exit()]
	rows := make([]Row, len(exit))
	for i, row := range exit {
		rows[i] = Row{Guard: row.Guard, Output: row.Output, Ops: row.Operations, Effects: row.Effects, Proofs: row.Proofs}
	}
	relation, err := prepared.builder.Build(prepared.certificate, rows)
	if err != nil {
		return Relation{}, err
	}
	return relation, nil
}

func BenchmarkPreparedPlanCompilerDenseDAG(b *testing.B) {
	prepared := preparedLinearBenchmark(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		relation := prepared.Evaluate()
		if relation.ContextualReason() != "" {
			b.Fatal(relation.ContextualReason())
		}
	}
}

func BenchmarkPreparedPlanCompilerLegacyAcyclic(b *testing.B) {
	prepared := preparedLinearBenchmark(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := evaluatePreparedAcyclicLegacyRaw(prepared, RelationView{}, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func preparedLinearBenchmark(b *testing.B) *PreparedPlanCompiler {
	b.Helper()
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, _ := factflow.NewStringLiteralValueSource("prepared", 0, 0, 0, shape)
	plan := operationplan.New(graph, factflow.FactsInput{
		Returns: map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{source})},
	})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{})
	if err != nil {
		b.Fatal(err)
	}
	return prepared
}
