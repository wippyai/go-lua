package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestScalarLocalAssignmentTransformerMatchesConcreteReturnBoundary(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f() local answer = 42 return answer end")
	prepared, err := PrepareFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}

	relation := transformer.NewPlanCompiler().Compile(reg, prepared.cfg.Graph, prepared.operationPlan, transformer.Shape{})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("scalar local relation compiled contextually: %s", reason)
	}
	cursor, err := transformer.NewBindingCursor(transformer.Shape{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, exact := relation.Specialize(cursor, nil, nil)
	if !exact {
		t.Fatal("scalar local relation did not specialize")
	}

	concrete := solvePreparedForTest(t, prepared, SolveConfig{})
	exit, ok := concrete.ExitState()
	if !ok {
		t.Fatal("concrete solve has no exit state")
	}
	want := summary.Normalize(reg, summary.Summary{Returns: []product.Value{exit.ReadReturnSlot(reg, 0)}})
	if !summary.Equal(reg, got, want) {
		t.Fatalf("symbolic/concrete return Summary differs\n got=%#v\nwant=%#v", got, want)
	}
	if gotDigest, wantDigest := summary.NormalizedPayloadDigest(reg, got), summary.NormalizedPayloadDigest(reg, want); gotDigest != wantDigest {
		t.Fatalf("symbolic/concrete normalized payload digest = %x/%x", gotDigest, wantDigest)
	}
	if lanes := state.DefaultLanes(); len(lanes) != 17 {
		t.Fatalf("state lane catalog = %d, want 17", len(lanes))
	}
	// This slice has one observable boundary lane (Returns/Values); the
	// compiler's certificate covers all 17 lanes and admits no other Summary
	// fact kind, so internal local-state changes cannot leak as invented output.
	if kinds := summary.PresentFactKinds(got); len(kinds) != 1 || kinds[0] != "Returns" {
		t.Fatalf("symbolic Summary fields = %v, want only Returns", kinds)
	}
}
