package program

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEvaluatedBoundChunkRunsTotalFourBodyRootChainWithoutLegacySolves(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(value: boolean) return value end
local function middle(value: boolean) local result = leaf(value); return result end
local function outer(value: boolean) local result = middle(value); return result end
local result = outer(true)
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("evaluated-four-body-root-chain"))
	stats := &Stats{}
	artifact, err := runEvaluatedBoundChunk(context.Background(), stmts, bindings, Config{
		Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}, UnitNamespace: namespace},
		Stats: stats,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Bodies()) != 4 || stats.EvaluatedShadowRootsProduced != 4 {
		t.Fatalf("evaluated bodies/roots = %d/%d, want 4/4", len(artifact.Bodies()), stats.EvaluatedShadowRootsProduced)
	}
	if stats.Body.StaticChunkPrepares != 1 || stats.Body.StaticFunctionPrepares != 3 {
		t.Fatalf("static chunk/function prepares = %d/%d, want 1/3", stats.Body.StaticChunkPrepares, stats.Body.StaticFunctionPrepares)
	}
	if stats.EvaluatedRelationCompilerPrepares != 4 {
		t.Fatalf("relation compiler prepares = %d, want 4", stats.EvaluatedRelationCompilerPrepares)
	}
	if stats.EvaluatedRelationEquationApplications != 4 || stats.PrebuiltSemanticLexicalEvaluations != 4 || stats.EvaluatedRootProjections != 4 {
		t.Fatalf("equations/evaluations/projections = %d/%d/%d, want 4/4/4",
			stats.EvaluatedRelationEquationApplications, stats.PrebuiltSemanticLexicalEvaluations, stats.EvaluatedRootProjections)
	}
	if stats.PrepassBodySolves != 0 || stats.SummaryBodySolves != 0 || stats.MaterializeBodySolves != 0 || stats.Body.BodySolves != 0 ||
		!reflect.DeepEqual(stats.Query, query.Stats{}) {
		t.Fatalf("legacy work entered evaluated invocation: prepass=%d summary=%d materialize=%d body=%d query=%#v",
			stats.PrepassBodySolves, stats.SummaryBodySolves, stats.MaterializeBodySolves, stats.Body.BodySolves, stats.Query)
	}
	for _, bodyID := range artifact.Bodies() {
		root, ok, err := artifact.Root(context.Background(), reg, bodyID)
		if err != nil || !ok || !root.Coverage().Complete() || stats.PrebuiltSemanticLexicalEvaluationsByBody[bodyID] != 1 {
			t.Fatalf("body %x root/evaluations = %#v/%d", bodyID, root, stats.PrebuiltSemanticLexicalEvaluationsByBody[bodyID])
		}
	}

	// The legacy invocation is a separate differential oracle. Its work must
	// never contaminate the counters above or become a fallback in the runner.
	legacyStats := &Stats{}
	legacy, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}, UnitNamespace: namespace},
		Stats: legacyStats, forceLegacyRelations: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.RootResult().ReleaseTransient()
	legacySummaries := make(map[lexicalidentity.StableLexicalBodyID]summary.Summary)
	collectLegacyEvaluatedSummaries(t, legacy.RootResult(), legacySummaries)
	for _, bodyID := range artifact.Bodies() {
		root, _, err := artifact.Root(context.Background(), reg, bodyID)
		if err != nil {
			t.Fatal(err)
		}
		want, ok := legacySummaries[bodyID]
		if !ok || !summary.Equal(reg, root.Summary(), want) {
			t.Fatalf("body %x evaluated summary differs from separate legacy oracle", bodyID)
		}
	}
	if stats.EvaluatedRelationEquationApplications != 4 || stats.EvaluatedRootProjections != 4 || !reflect.DeepEqual(stats.Query, query.Stats{}) {
		t.Fatal("separate legacy oracle mutated evaluated invocation counters")
	}
}

func TestEvaluatedBoundChunkCancellationAndUnsupportedInputPublishZero(t *testing.T) {
	stmts := parseChunk(t, `local function leaf(value: string): string return value end return leaf("value")`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats := &Stats{}
	artifact, err := runEvaluatedBoundChunk(ctx, stmts, bindings, Config{Check: body.Config{Registry: standard.Registry()}, Stats: stats})
	if !errors.Is(err, context.Canceled) || len(artifact.Bodies()) != 0 || stats.Body.StaticChunkPrepares != 0 || stats.EvaluatedShadowRootsProduced != 0 {
		t.Fatalf("canceled evaluated run = bodies:%d prepares:%d roots:%d err:%v",
			len(artifact.Bodies()), stats.Body.StaticChunkPrepares, stats.EvaluatedShadowRootsProduced, err)
	}

	unsupported := parseChunk(t, `
local suffix = "!"
local function leaf(value: string): string return value .. suffix end
return leaf("value")
`)
	unsupportedBindings := bind.BindChunk(unsupported, bind.Options{})
	unsupportedStats := &Stats{}
	artifact, err = runEvaluatedBoundChunk(context.Background(), unsupported, unsupportedBindings, Config{
		Check: body.Config{Registry: standard.Registry()}, Stats: unsupportedStats,
	})
	if err == nil || len(artifact.Bodies()) != 0 || unsupportedStats.EvaluatedShadowRootsProduced != 0 {
		t.Fatalf("unsupported evaluated run published artifact: bodies=%d roots=%d err=%v",
			len(artifact.Bodies()), unsupportedStats.EvaluatedShadowRootsProduced, err)
	}
}

func TestEvaluatedBoundChunkArtifactPreservesInvalidCallEvidenceExactly(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(value: string) return value end
local result = leaf(false)
return result
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("evaluated-invalid-call-evidence"))
	artifact, err := runEvaluatedBoundChunk(context.Background(), stmts, bindings, Config{
		Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}, UnitNamespace: namespace}, Stats: &Stats{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var argument, result, assignment, routes, obligations int
	for _, bodyID := range artifact.Bodies() {
		root, ok, err := artifact.Root(context.Background(), reg, bodyID)
		if err != nil || !ok {
			t.Fatalf("materialize body %x: ok=%v err=%v", bodyID, ok, err)
		}
		for _, slot := range root.Observations() {
			for _, item := range slot.Observed {
				if item.Worlds.Root == evaluated.DecisionFalse || item.Owner == (lexicalidentity.StableLexicalBodyID{}) ||
					!item.Anchor.Valid() || item.Anchor.Kind != item.Kind || item.Anchor.Slot != item.Slot {
					t.Fatalf("invalid durable observation identity: %#v", item)
				}
				switch item.Kind {
				case observation.CallArgument:
					if !item.HasExpected || !product.Equal(reg, item.Actual, typevalue.LiteralBool(reg, false)) ||
						!product.Equal(reg, item.Expected, typevalue.String(reg)) {
						t.Fatalf("call argument evidence = %#v", item)
					}
					argument++
				case observation.CallResult:
					result++
				case observation.Assignment:
					assignment++
				}
			}
			for _, owed := range slot.Obligations {
				if owed.Worlds.Root == evaluated.DecisionFalse || owed.Owner == (lexicalidentity.StableLexicalBodyID{}) || !owed.Anchor.Valid() {
					t.Fatalf("invalid durable observation obligation: %#v", owed)
				}
				obligations++
			}
		}
		for _, route := range root.Routes() {
			if route.Worlds.Root == evaluated.DecisionFalse || !route.Anchor.Valid() || route.Anchor.Kind != observation.CallInvocation {
				t.Fatalf("invalid durable invocation route: %#v", route)
			}
			routes++
		}
		again, ok, err := artifact.Root(context.Background(), reg, bodyID)
		if err != nil || !ok {
			t.Fatalf("repeat materialize body %x: ok=%v err=%v", bodyID, ok, err)
		}
		assertEvaluatedObservationsEqual(t, reg, root.Observations(), again.Observations())
	}
	if argument == 0 || result == 0 || assignment == 0 || routes == 0 || obligations == 0 {
		t.Fatalf("durable evidence argument/result/assignment/routes/obligations = %d/%d/%d/%d/%d",
			argument, result, assignment, routes, obligations)
	}
}

func assertEvaluatedObservationsEqual(t *testing.T, reg *axis.Registry, left, right []evaluated.ObservationSlot) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("materialized observation slots = %d/%d", len(left), len(right))
	}
	for index := range left {
		if left[index].Slot != right[index].Slot || left[index].Point != right[index].Point ||
			len(left[index].Observed) != len(right[index].Observed) || len(left[index].Obligations) != len(right[index].Obligations) {
			t.Fatalf("materialized observation slot %d structure differs", index)
		}
		for itemIndex := range left[index].Observed {
			a, b := left[index].Observed[itemIndex], right[index].Observed[itemIndex]
			aActual, aExpected, bActual, bExpected := a.Actual, a.Expected, b.Actual, b.Expected
			a.Actual, a.Expected, b.Actual, b.Expected = product.Value{}, product.Value{}, product.Value{}, product.Value{}
			if !reflect.DeepEqual(a, b) || !product.Equal(reg, aActual, bActual) || a.HasExpected && !product.Equal(reg, aExpected, bExpected) {
				t.Fatalf("materialized observation %d/%d differs", index, itemIndex)
			}
		}
		if !reflect.DeepEqual(left[index].Obligations, right[index].Obligations) {
			t.Fatalf("materialized obligations at slot %d differ", index)
		}
	}
}
