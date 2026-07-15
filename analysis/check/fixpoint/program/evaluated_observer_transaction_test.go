package program

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEvaluatedObserverTransactionProjectsOnlyUniqueEntry(t *testing.T) {
	catalog := captureRelationCatalog(t, `
local function first(value: string) return value end
local function second(value: string) return value end
local function third(value: string) return value end
`)
	if len(catalog.entries) != 3 {
		t.Fatalf("catalog templates = %d, want 3", len(catalog.entries))
	}
	// The production total catalog owns a nil-function chunk root. This sealed
	// lexical fixture reuses one independent equation as that unique test root;
	// the other two templates exercise the uncalled validation partition. The
	// equation identity and every call route remain unchanged.
	catalog.entries[0].function = nil
	forest, err := buildLexicalObserverForest(context.Background(), catalog)
	if err != nil {
		t.Fatal(err)
	}

	reg := standard.Registry()
	boundary := make(map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings, len(catalog.entries))
	for _, entry := range catalog.entries {
		shape := entry.compiler.Shape()
		values := make([]product.Value, shape.ValueCount())
		for index := range values {
			values[index] = typevalue.String(reg)
		}
		plan := entry.identity.Prepared.OperationPlan()
		order := append([]symbol.ID(nil), plan.BoundaryParams()...)
		order = append(order, plan.BoundaryCaptures()...)
		order = append(order, plan.BoundaryGlobals()...)
		boundary[lexicalBodyForEvaluatedEntry(entry)] = evaluatedProgramBindings{values: values, order: order}
	}
	stats := &Stats{}
	artifact, err := solveEvaluatedObserverProgram(context.Background(), catalog, boundary, forest, stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Bodies()) != 1 || stats.EvaluatedShadowRootsProduced != 1 || stats.EvaluatedRootProjections != 1 {
		t.Fatalf("entry bodies/roots/projections = %d/%d/%d, want 1/1/1",
			len(artifact.Bodies()), stats.EvaluatedShadowRootsProduced, stats.EvaluatedRootProjections)
	}
	if stats.EvaluatedRelationEquationApplications != 3 || stats.PrebuiltSemanticLexicalEvaluations != 3 ||
		stats.EvaluatedObserverDiagnosticNodes != 1 || stats.EvaluatedObserverDiagnosticEdges != 0 ||
		stats.EvaluatedObserverUncalledTemplates != 2 ||
		stats.EvaluatedObserverProgramPublications != 1 {
		t.Fatalf("equations/evaluations/nodes/edges/uncalled/publications = %d/%d/%d/%d/%d/%d, want 3/3/1/0/2/1",
			stats.EvaluatedRelationEquationApplications, stats.PrebuiltSemanticLexicalEvaluations,
			stats.EvaluatedObserverDiagnosticNodes, stats.EvaluatedObserverDiagnosticEdges,
			stats.EvaluatedObserverUncalledTemplates,
			stats.EvaluatedObserverProgramPublications)
	}
	if stats.PrepassBodySolves != 0 || stats.SummaryBodySolves != 0 || stats.MaterializeBodySolves != 0 ||
		stats.Body.BodySolves != 0 || !reflect.DeepEqual(stats.Query, query.Stats{}) {
		t.Fatalf("observer transaction entered a legacy solve: prepass=%d summary=%d materialize=%d body=%d query=%#v",
			stats.PrepassBodySolves, stats.SummaryBodySolves, stats.MaterializeBodySolves, stats.Body.BodySolves, stats.Query)
	}
	entry, ok, err := artifact.Entry(context.Background(), reg)
	if err != nil || !ok || entry.Identity().Body != forest.Root.Body {
		t.Fatalf("unique entry = body:%x ok:%v err:%v, want %x", entry.Identity().Body, ok, err, forest.Root.Body)
	}
}
