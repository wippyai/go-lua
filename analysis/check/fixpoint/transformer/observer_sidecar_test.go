package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCFGRowDedupUnionsObservationsOutsideSemanticBudget(t *testing.T) {
	reg := standard.Registry()
	builder, _ := emptyBuilder(t, reg, Shape{}, nil)
	arena := builder.Arena()
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	actual := arena.Constant(typevalue.LiteralString(reg, "actual"))
	first := sidecarObservation(arena, actual, actual, 1)
	second := sidecarObservation(arena, actual, actual, 2)

	rows, err := SolveAcyclicCFGExpandedRows(graph, arena,
		SymbolicCFGRow{Guard: arena.True()},
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			if point != graph.Entry() {
				return []SymbolicCFGRow{row}, nil
			}
			left, right := cloneCFGRow(row), cloneCFGRow(row)
			left.Observations = []ObservationTerm{second}
			right.Observations = []ObservationTerm{first}
			return []SymbolicCFGRow{left, right}, nil
		}, nil, SymbolicCFGOptions{MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	exit := rows[graph.Exit()]
	if len(exit) != 1 || len(exit[0].Observations) != 2 {
		t.Fatalf("exit semantic/annotation rows = %d/%#v", len(exit), exit)
	}
	if exit[0].Observations[0] != first || exit[0].Observations[1] != second {
		t.Fatalf("annotations are not canonical: %#v", exit[0].Observations)
	}
}

func TestRelationObservationSidecarIsSemanticallyInvisibleAndACI(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{}, nil)
	arena := builder.Arena()
	actualA := arena.Constant(typevalue.LiteralString(reg, "actual-a"))
	expectedA := arena.Constant(typevalue.LiteralString(reg, "expected-a"))
	actualB := arena.Constant(typevalue.LiteralString(reg, "actual-b"))
	expectedB := arena.Constant(typevalue.LiteralString(reg, "expected-b"))
	first := sidecarObservation(arena, actualA, expectedA, 1)
	second := sidecarObservation(arena, actualB, expectedB, 2)
	base, err := builder.Build(certificate, []Row{{Guard: arena.True()}})
	if err != nil {
		t.Fatal(err)
	}
	enriched, err := builder.Build(certificate, []Row{
		{Guard: arena.True(), Observations: []ObservationTerm{second}},
		{Guard: arena.True(), Observations: []ObservationTerm{first}},
		{Guard: arena.True(), Observations: []ObservationTerm{second, first}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if base.Rows() != 1 || enriched.Rows() != 1 || !EqualRelation(base, enriched) ||
		!LessOrEqRelation(base, enriched) || !LessOrEqRelation(enriched, base) {
		t.Fatalf("annotation changed relation semantics: base=%d enriched=%d equal=%v", base.Rows(), enriched.Rows(), EqualRelation(base, enriched))
	}
	left := JoinRelation(base, enriched)
	right := JoinRelation(enriched, base)
	if got := WidenRelation(base, enriched, 1); got.ContextualReason() != "" || got.Rows() != 1 {
		t.Fatalf("annotation consumed row budget: reason=%q rows=%d", got.ContextualReason(), got.Rows())
	}
	for name, relation := range map[string]Relation{"left": left, "right": right} {
		if len(relation.rows) != 1 || len(relation.rows[0].Observations) != 2 ||
			relation.rows[0].Observations[0] != first || relation.rows[0].Observations[1] != second {
			t.Fatalf("%s annotation union is not ACI/canonical: %#v", name, relation.rows)
		}
	}
	if rowKey(arena, builder.effects, base.rows[0]) != rowKey(arena, builder.effects, enriched.rows[0]) {
		t.Fatal("annotation changed structural semantic-row witness")
	}

	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	detailed, exact := enriched.SpecializeDetailed(cursor, nil, SpecializationContext{})
	items := detailed.Observations.Items()
	if !exact || len(items) != 2 {
		t.Fatalf("specialized annotations = %#v exact=%v", items, exact)
	}
	pairs := map[string]string{}
	for _, item := range items {
		actual, _ := typevalue.StringLiteralOf(reg, item.Actual)
		expected, _ := typevalue.StringLiteralOf(reg, item.Expected)
		pairs[actual] = expected
	}
	if pairs["actual-a"] != "expected-a" || pairs["actual-b"] != "expected-b" {
		t.Fatalf("expected/actual correlation torn: %#v", pairs)
	}
}

func TestObservationCoverageMetadataIsNotRelationSemantics(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{}, nil)
	base, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True()}})
	if err != nil {
		t.Fatal(err)
	}
	complete := base
	complete.observationComplete = true
	if !EqualRelation(base, complete) || !LessOrEqRelation(base, complete) || !LessOrEqRelation(complete, base) {
		t.Fatal("observation coverage metadata changed relation semantics")
	}
	joined := JoinRelation(complete, base)
	if joined.ContextualReason() != "" || joined.ObservationCoverageComplete() {
		t.Fatalf("coverage join must fail closed without becoming contextual: %#v", joined)
	}
}

func TestRelationSCCObservationOnlyGrowthDoesNotRestartSemantics(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{}, nil)
	arena := builder.Arena()
	actual := arena.Constant(typevalue.LiteralString(reg, "actual"))
	base, err := builder.Build(certificate, []Row{{Guard: arena.True(), Observations: []ObservationTerm{sidecarObservation(arena, actual, actual, 1)}}})
	if err != nil {
		t.Fatal(err)
	}
	enriched, err := builder.Build(certificate, []Row{{Guard: arena.True(), Observations: []ObservationTerm{
		sidecarObservation(arena, actual, actual, 1), sidecarObservation(arena, actual, actual, 2),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	ref := CellRef{Function: 91}
	calls := 0
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{{
		Ref: ref, Arena: arena, Shape: Shape{}, Dependencies: []CellRef{ref},
		Equation: func(context.Context, RelationView) (Relation, error) {
			calls++
			if calls == 1 {
				return base, nil
			}
			return enriched, nil
		},
	}}, RelationSolveOptions{MaxRows: 1, MaxIterations: 8})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snapshot.Lookup(ref)
	if !ok || calls != 2 || got.Rows() != 1 || len(got.rows[0].Observations) != 2 {
		t.Fatalf("evidence-only SCC growth restarted or was lost: ok=%v calls=%d relation=%#v", ok, calls, got)
	}
}

func sidecarObservation(arena *Arena, actual, expected ValueTerm, line uint32) ObservationTerm {
	return ObservationTerm{
		BodyOwner: testObservationBody(byte(line)),
		Kind:      ObservationCallResult,
		Anchor:    testObservationAnchor(ObservationCallResult, line, 0),
		Guard:     arena.True(),
		Actual:    actual,
		Expected:  expected,
	}
}
