package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPlanCompilerDirectScalarReturnSpecializesExactly(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)

	shape, ok := factflow.NewValueSourceShape(false, false, false, false)
	if !ok {
		t.Fatal("scalar source shape rejected")
	}
	literal, ok := factflow.NewIntegerLiteralValueSource(42, 0, 0, 0, shape)
	if !ok {
		t.Fatal("literal source rejected")
	}
	ref := factflow.ExprRef(1)
	expression, ok := factflow.NewExpressionValueSource(ref, 1, 1, 0, shape)
	if !ok {
		t.Fatal("expression source rejected")
	}
	wantString := typevalue.LiteralString(reg, "ok")
	plan := operationplan.New(graph, factflow.FactsInput{
		Returns:          map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{literal, expression})},
		ExpressionValues: map[factflow.ExprRef]product.Value{ref: wantString},
	})

	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("direct return compiled contextually: %s", reason)
	}
	if relation.Rows() != 1 {
		t.Fatalf("relation rows = %d, want 1", relation.Rows())
	}
	cursor, err := NewBindingCursor(Shape{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, exact := relation.Specialize(cursor, nil, nil)
	if !exact {
		t.Fatal("direct relation did not specialize")
	}
	want := summary.Normalize(reg, summary.Summary{Returns: []product.Value{
		typevalue.LiteralInt(reg, 42), wantString,
	}})
	if !summary.Equal(reg, got, want) {
		t.Fatalf("specialized Summary differs\n got=%#v\nwant=%#v", got, want)
	}
}

func TestPlanCompilerUnsupportedFamiliesFailAsOneContextualRelation(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	plan := operationplan.New(graph, factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{point: {}},
		CallSites:       map[cfg.Point]factflow.CallSite{point: {}},
	})

	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	reason := relation.ContextualReason()
	const wantReason = "compiler: contextual operations: CallSites, RootAssignments"
	if reason != wantReason {
		t.Fatalf("contextual reason = %q, want deterministic aggregate %q", reason, wantReason)
	}
	if relation.Rows() != 0 {
		t.Fatalf("contextual relation published %d partial rows", relation.Rows())
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	if got, ok := relation.Specialize(cursor, nil, nil); ok || len(got.Returns) != 0 {
		t.Fatalf("contextual relation specialized partial output: ok=%v got=%#v", ok, got)
	}
}

func TestPlanCompilerExpressionValueWithContextualSidecarFailsClosed(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	ref := factflow.ExprRef(1)
	source, ok := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
	if !ok {
		t.Fatal("expression source rejected")
	}
	value := typevalue.LiteralString(reg, "narrowed")
	plan := operationplan.New(graph, factflow.FactsInput{
		Returns:               map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{source})},
		ExpressionValues:      map[factflow.ExprRef]product.Value{ref: value},
		ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{ref: factflow.NewExpressionRefinement(source, value)},
	})

	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	if got := relation.ContextualReason(); got != "compiler: contextual operations: ExpressionRefinements" {
		t.Fatalf("contextual sidecar reason = %q", got)
	}
	if relation.Rows() != 0 {
		t.Fatalf("contextual sidecar published %d rows", relation.Rows())
	}
}

func TestPlanCompilerRegistryTracksEntireOperationCatalog(t *testing.T) {
	compiler := NewPlanCompiler()
	if len(compiler.facts) != len(operationplan.Kinds()) {
		t.Fatalf("fact registrations=%d catalog=%d", len(compiler.facts), len(operationplan.Kinds()))
	}
	for _, fact := range operationplan.Kinds() {
		if _, registered := compiler.facts[fact]; !registered {
			t.Fatalf("operation-plan kind %s has no explicit compiler verdict", fact)
		}
		if handler := compiler.facts[fact]; handler != nil && handler.Kind() != fact {
			t.Fatalf("operation-plan kind %s registered handler for %s", fact, handler.Kind())
		}
	}
	if len(compiler.extensions) != len(operationplan.ExtensionKinds()) {
		t.Fatalf("extension registrations=%d catalog=%d", len(compiler.extensions), len(operationplan.ExtensionKinds()))
	}
	for _, extension := range operationplan.ExtensionKinds() {
		if _, registered := compiler.extensions[extension]; !registered {
			t.Fatalf("operation-plan extension %d has no explicit compiler verdict", extension)
		}
	}
}
