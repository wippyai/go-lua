package program

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEvaluatedProgramEvaluatesPrebuiltLexicalCatalogOncePerTransaction(t *testing.T) {
	stmts := parseChunk(t, `
local function first() end
local function second() end
local function third() end
first(); second(); third()
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	var catalog relationRunCatalog
	legacy, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg},
		// The concrete run is only the differential oracle. It is not reachable
		// from solveEvaluatedProgram and cannot become a per-owner fallback.
		forceLegacyRelations: true,
		relationCatalogAudit: func(got relationRunCatalog) error {
			catalog = got
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.RootResult().ReleaseTransient()
	if len(catalog.entries) != 3 {
		t.Fatalf("admitted lexical bodies = %d, want 3", len(catalog.entries))
	}

	boundary := make(map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings, len(catalog.entries))
	for _, entry := range catalog.entries {
		bodyID := lexicalBodyForEvaluatedEntry(entry)
		boundary[bodyID] = evaluatedProgramBindings{}
	}
	stats := &Stats{}
	first, err := solveEvaluatedProgram(context.Background(), catalog, boundary, stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Bodies()) != 3 || stats.PrebuiltSemanticLexicalEvaluations != 3 || stats.EvaluatedShadowRootsProduced != 3 {
		t.Fatalf("bodies/evaluations/roots = %d/%d/%d, want 3/3/3", len(first.Bodies()), stats.PrebuiltSemanticLexicalEvaluations, stats.EvaluatedShadowRootsProduced)
	}
	for _, bodyID := range first.Bodies() {
		if got := stats.PrebuiltSemanticLexicalEvaluationsByBody[bodyID]; got != 1 {
			t.Fatalf("body %x semantic evaluations = %d, want exactly one", bodyID, got)
		}
	}
	if stats.PrepassBodySolves != 0 || stats.SummaryBodySolves != 0 || stats.MaterializeBodySolves != 0 || stats.Body.BodySolves != 0 {
		t.Fatalf("legacy solves entered evaluated transaction: prepass=%d summary=%d materialize=%d body=%d",
			stats.PrepassBodySolves, stats.SummaryBodySolves, stats.MaterializeBodySolves, stats.Body.BodySolves)
	}

	legacySummaries := make(map[lexicalidentity.StableLexicalBodyID]summary.Summary)
	collectLegacyEvaluatedSummaries(t, legacy.RootResult(), legacySummaries)
	for _, bodyID := range first.Bodies() {
		root, ok, err := first.Root(context.Background(), reg, bodyID)
		if err != nil || !ok || !root.Coverage().Complete() {
			t.Fatalf("body %x has no complete evaluated root", bodyID)
		}
		want, ok := legacySummaries[bodyID]
		if !ok {
			t.Fatalf("legacy oracle has no body %x", bodyID)
		}
		if !summary.Equal(reg, root.Summary(), want) {
			t.Fatalf("body %x summary differs from concrete oracle", bodyID)
		}
		gotCanonical, err := summary.EncodeCanonical(context.Background(), reg, root.Summary())
		if err != nil {
			t.Fatal(err)
		}
		wantCanonical, err := summary.EncodeCanonical(context.Background(), reg, want)
		if err != nil {
			t.Fatal(err)
		}
		if gotCanonical.Schema != wantCanonical.Schema || gotCanonical.Semantic != wantCanonical.Semantic || !bytes.Equal(gotCanonical.Bytes, wantCanonical.Bytes) {
			t.Fatalf("body %x canonical summary authority differs from concrete oracle", bodyID)
		}
	}

	// A second transaction must freeze the exact same compact projection,
	// including point/edge/observation world products. It receives fresh stats
	// so per-run counts cannot accidentally describe cumulative cache reuse.
	secondStats := &Stats{}
	second, err := solveEvaluatedProgram(context.Background(), catalog, boundary, secondStats)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("evaluated summary/observation projection changed across deterministic repeat")
	}
	for _, count := range secondStats.PrebuiltSemanticLexicalEvaluationsByBody {
		if count != 1 {
			t.Fatalf("repeat semantic evaluation count = %d, want one", count)
		}
	}
}

func TestEvaluatedProgramCancellationPublishesNothing(t *testing.T) {
	stmts := parseChunk(t, `local function leaf(): string return "ok" end return leaf()`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	var catalog relationRunCatalog
	_, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: standard.Registry()}, forceLegacyRelations: true,
		relationCatalogAudit: func(got relationRunCatalog) error { catalog = got; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary := make(map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings)
	for _, entry := range catalog.entries {
		boundary[lexicalBodyForEvaluatedEntry(entry)] = evaluatedProgramBindings{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats := &Stats{}
	artifact, err := solveEvaluatedProgram(ctx, catalog, boundary, stats)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled transaction error = %v, want context.Canceled", err)
	}
	if len(artifact.Bodies()) != 0 || stats.EvaluatedShadowRootsProduced != 0 || stats.PrebuiltSemanticLexicalEvaluations != 0 {
		t.Fatalf("canceled transaction returned bodies/roots/evaluations = %d/%d/%d",
			len(artifact.Bodies()), stats.EvaluatedShadowRootsProduced, stats.PrebuiltSemanticLexicalEvaluations)
	}
}

func TestEvaluatedProgramRejectsCrossNamespaceBindingPermutation(t *testing.T) {
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{11}).
		WithBoundaryCaptures([]symbol.ID{22}).
		WithBoundaryGlobals([]symbol.ID{33})
	correct := evaluatedProgramBindings{order: []symbol.ID{11, 22, 33}}
	if !evaluatedBindingOrderMatchesPlan(plan, correct) {
		t.Fatal("exact parameter/capture/global binding order was rejected")
	}
	permuted := evaluatedProgramBindings{order: []symbol.ID{33, 11, 22}}
	if evaluatedBindingOrderMatchesPlan(plan, permuted) {
		t.Fatal("cross-namespace parameter/capture/global permutation was admitted")
	}
}

func TestEvaluatedProgramDirectCallChainUsesExactResultRoots(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(value: string, other: string) return value end
local function middle(value: string, other: string) local result = leaf(value, other); return result end
local function outer(value: string, other: string) local result = middle(value, other); return result end
local result = outer("value", "other")
return result
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	var catalog relationRunCatalog
	legacy, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}}, forceLegacyRelations: true,
		relationCatalogAudit: func(got relationRunCatalog) error { catalog = got; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.RootResult().ReleaseTransient()
	if len(catalog.entries) != 3 {
		t.Fatalf("direct-call lexical bodies = %d, want 3", len(catalog.entries))
	}
	boundary := make(map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings)
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
	artifact, err := solveEvaluatedProgram(context.Background(), catalog, boundary, stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Bodies()) != 3 || stats.PrebuiltSemanticLexicalEvaluations != 3 || stats.EvaluatedShadowRootsProduced != 3 {
		t.Fatalf("direct-call bodies/evaluations/roots = %d/%d/%d, want 3/3/3",
			len(artifact.Bodies()), stats.PrebuiltSemanticLexicalEvaluations, stats.EvaluatedShadowRootsProduced)
	}
	legacySummaries := make(map[lexicalidentity.StableLexicalBodyID]summary.Summary)
	collectLegacyEvaluatedSummaries(t, legacy.RootResult(), legacySummaries)
	callProducerBoundaries := 0
	for _, bodyID := range artifact.Bodies() {
		root, _, err := artifact.Root(context.Background(), reg, bodyID)
		if err != nil {
			t.Fatal(err)
		}
		want := legacySummaries[bodyID]
		if !summary.Equal(reg, root.Summary(), want) {
			t.Fatalf("body %x summary differs from concrete oracle\nevaluated: %#v\nconcrete:  %#v", bodyID, root.Summary(), want)
		}
		gotCanonical, err := summary.EncodeCanonical(context.Background(), reg, root.Summary())
		if err != nil {
			t.Fatal(err)
		}
		wantCanonical, err := summary.EncodeCanonical(context.Background(), reg, want)
		if err != nil {
			t.Fatal(err)
		}
		if gotCanonical.Schema != wantCanonical.Schema || gotCanonical.Semantic != wantCanonical.Semantic || !bytes.Equal(gotCanonical.Bytes, wantCanonical.Bytes) {
			t.Fatalf("body %x canonical summary authority differs from concrete oracle", bodyID)
		}
		for _, boundary := range root.Boundaries() {
			for _, fragment := range boundary.Fragments {
				if len(fragment.Values) == 1 && product.Equal(reg, fragment.Values[0].Value, typevalue.String(reg)) {
					callProducerBoundaries++
				}
			}
		}
	}
	if callProducerBoundaries < 2 {
		t.Fatalf("exact string call-producer boundaries = %d, want at least two wrappers", callProducerBoundaries)
	}

	// Binding order is semantic authority, not a caller convention.
	for bodyID, binding := range boundary {
		if len(binding.order) < 2 {
			continue
		}
		forged := make(map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings, len(boundary))
		for key, value := range boundary {
			forged[key] = value
		}
		changed := binding
		changed.order = append([]symbol.ID(nil), binding.order...)
		changed.order[0], changed.order[1] = changed.order[1], changed.order[0]
		forged[bodyID] = changed
		if partial, err := solveEvaluatedProgram(context.Background(), catalog, forged, &Stats{}); err == nil || len(partial.Bodies()) != 0 {
			t.Fatalf("body %x forged boundary order was admitted", bodyID)
		}
		break
	}
}

func TestEvaluatedProgramRejectsForgedCatalogProducersTransactionally(t *testing.T) {
	stmts := parseChunk(t, `
local function first(value: string) return value end
local function second(value: string) return value end
local function caller(value: string) return first(value) end
return caller(second("value"))
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	var catalog relationRunCatalog
	_, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}}, forceLegacyRelations: true,
		relationCatalogAudit: func(got relationRunCatalog) error { catalog = got; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.entries) < 2 {
		t.Fatalf("catalog entries = %d, want at least two", len(catalog.entries))
	}
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
	assertRejected := func(name string, mutate func(*relationRunCatalog)) {
		t.Helper()
		forged := catalog
		forged.entries = append([]relationCatalogEntry(nil), catalog.entries...)
		mutate(&forged)
		stats := &Stats{}
		artifact, err := solveEvaluatedProgram(context.Background(), forged, boundary, stats)
		if err == nil || len(artifact.Bodies()) != 0 || stats.EvaluatedShadowRootsProduced != 0 {
			t.Fatalf("%s admitted partial/forged transaction: bodies=%d roots=%d err=%v", name, len(artifact.Bodies()), stats.EvaluatedShadowRootsProduced, err)
		}
	}
	assertRejected("body-digest", func(c *relationRunCatalog) { c.entries[0].identity.BodyDigest++ })
	assertRejected("foreign-compiler", func(c *relationRunCatalog) { c.entries[0].compiler = c.entries[1].compiler })
	assertRejected("summary-owner", func(c *relationRunCatalog) { c.entries[0].identity.Summary = c.entries[1].identity.Summary })
	assertRejected("same-shape-route-redirect", func(c *relationRunCatalog) {
		for index := range c.entries {
			entry := &c.entries[index]
			if len(entry.direct.Cells()) == 0 {
				continue
			}
			routes := make(map[cfg.Point]transformer.DirectCallTarget)
			redirected := false
			for raw := 0; raw < entry.direct.PointCount(); raw++ {
				point := cfg.Point(raw)
				target, ok := entry.direct.Lookup(point)
				if !ok {
					continue
				}
				if !redirected {
					for _, alternate := range c.entries {
						if alternate.identity.Cell != entry.identity.Cell && alternate.identity.Cell != target.Cell && alternate.compiler.Shape() == target.Shape {
							target.Cell = alternate.identity.Cell
							redirected = true
							break
						}
					}
				}
				routes[point] = target
			}
			if !redirected {
				t.Fatal("fixture has no same-shape alternate relation cell")
			}
			forged, err := transformer.NewDirectCallCatalog(entry.direct.PointCount(), routes)
			if err != nil {
				t.Fatal(err)
			}
			entry.direct = forged
			return
		}
		t.Fatal("fixture has no direct-call producer")
	})
}

func TestEvaluatedProgramSealsStringLaneThroughCanonicalArtifact(t *testing.T) {
	stmts := parseChunk(t, `local function leaf() return "value" end local value = leaf(); return value`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	var catalog relationRunCatalog
	_, err := RunBoundChunk(stmts, bindings, Config{
		Check: body.Config{Registry: reg}, forceLegacyRelations: true,
		relationCatalogAudit: func(got relationRunCatalog) error { catalog = got; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary := make(map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings)
	for _, entry := range catalog.entries {
		boundary[lexicalBodyForEvaluatedEntry(entry)] = evaluatedProgramBindings{}
	}
	stats := &Stats{}
	artifact, err := solveEvaluatedProgram(context.Background(), catalog, boundary, stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Bodies()) != len(catalog.entries) || stats.EvaluatedShadowRootsProduced != len(catalog.entries) {
		t.Fatalf("string artifact bodies/roots = %d/%d, want %d", len(artifact.Bodies()), stats.EvaluatedShadowRootsProduced, len(catalog.entries))
	}
	for _, bodyID := range artifact.Bodies() {
		root, ok, err := artifact.Root(context.Background(), reg, bodyID)
		if err != nil || !ok || !root.Coverage().Complete() {
			t.Fatalf("body %x string artifact materialization = ok:%v coverage:%#v err:%v", bodyID, ok, root.Coverage(), err)
		}
	}
}

func collectLegacyEvaluatedSummaries(t *testing.T, result *body.Result, out map[lexicalidentity.StableLexicalBodyID]summary.Summary) {
	t.Helper()
	if result == nil {
		return
	}
	if bodyID := result.StableLexicalBodyID(); bodyID != (lexicalidentity.StableLexicalBodyID{}) {
		projected, err := summaryprojection.FromResultContext(context.Background(), result)
		if err != nil {
			t.Fatal(err)
		}
		out[bodyID] = projected
	}
	for _, child := range result.FunctionResults() {
		collectLegacyEvaluatedSummaries(t, child, out)
	}
}
