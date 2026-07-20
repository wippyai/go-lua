package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPathStorePlanHandlerPublishesOneIndependentTransaction(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	table, sourceID := symbol.ID(9701), symbol.ID(9702)
	target := pathdom.NewPath(table, "table").Field("value")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 9702, HasExpr: true}
	input := factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			point: factflow.NewPathAssignment(target, source),
		},
		PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
			point: factflow.NewPathStaticMemberWrite(target, source),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{source.ExprRef: pathdom.NewPath(sourceID, "source")},
	}
	plan := operationplan.New(graph, input).WithBoundaryParams([]symbol.ID{table, sourceID})
	builder := NewBuilder(reg, Shape{Params: 2}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder, locals: make(map[symbol.ID]ValueTerm), expressions: make(map[factflow.ExprRef][]ValueTerm)}
	if err := bindBoundaryParamTerms(&ctx, Shape{Params: 2}); err != nil {
		t.Fatal(err)
	}
	var steps []rowStep
	ctx.rowSteps = &steps
	assignment := pathStorePlanHandler{kind: operationplan.PathAssignment}
	static := pathStorePlanHandler{kind: operationplan.PathStaticMemberWrite}
	if err := assignment.Preflight(ctx, point); err != nil {
		t.Fatal(err)
	}
	if err := static.Preflight(ctx, point); err != nil {
		t.Fatal(err)
	}
	if err := assignment.Lower(ctx, point, nil); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Fatal("path assignment did not publish the complete N4 transaction")
	}
	if err := static.Lower(ctx, point, nil); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].kind != rowStepEffect || builder.EffectArena().Kind(steps[0].effect) != EffectPathStore {
		t.Fatalf("steps = %#v, want one path-store effect", steps)
	}
	if !builder.EffectArena().Valid(steps[0].effect, Shape{Params: 2}) {
		t.Fatal("path-store effect does not own its boundary terms")
	}
	cursor, err := NewBindingCursor(Shape{Params: 2}, []product.Value{product.Top(), product.Top()}, []pathdom.Path{pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1)})
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := builder.EffectArena().resolve(steps[0].effect, cursor, SpecializationContext{})
	if !ok || resolved.Kind != EffectPathStore || !resolved.PathStore.HasAssignment || !resolved.PathStore.HasStatic ||
		!resolved.PathStore.Assignment.Target.Equal(pathdom.NewPlaceholder(0).Field("value")) ||
		!resolved.PathStore.Assignment.SourcePath.Equal(pathdom.NewPlaceholder(1)) {
		t.Fatalf("resolved path store = %#v/%t", resolved, ok)
	}
}

func TestPathStorePlanHandlerFreezesNestedObjectSidecar(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	table, sourceID := symbol.ID(9711), symbol.ID(9712)
	rootRef, nestedRef := factflow.ExprRef(9713), factflow.ExprRef(9714)
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: rootRef, HasExpr: true}
	nestedSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: nestedRef, HasExpr: true}
	leafSource := factflow.ValueSource{Kind: factflow.ValueSourceLiteral, LiteralKind: factflow.ValueSourceLiteralString, String: "leaf"}
	target := pathdom.NewPath(table, "table").Field("value")
	indexOne := pathdom.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: 1}}}
	child := pathdom.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "child"}}}
	id := pathdom.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "id"}}}
	input := factflow.FactsInput{
		PathAssignments:        map[cfg.Point]factflow.PathAssignment{point: factflow.NewPathAssignment(target, rootSource)},
		PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{point: factflow.NewPathStaticMemberWrite(target, rootSource)},
		ExpressionPaths:        map[factflow.ExprRef]pathdom.Path{rootRef: pathdom.NewPath(sourceID, "source")},
		ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
			rootRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(indexOne, leafSource, factflow.SourceSpan{}, "").WithExpected(product.Top()),
				factflow.NewObjectEntryWithMetadata(child, nestedSource, factflow.SourceSpan{}, ""),
			}).WithIdentity(identity.ID{Kind: "table", Site: "compiler-path-store", Index: uint64(rootRef)}),
			nestedRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(id, leafSource, factflow.SourceSpan{}, ""),
			}).WithIdentity(identity.ID{Kind: "table", Site: "compiler-path-store", Index: uint64(nestedRef)}),
		},
	}
	plan := operationplan.New(graph, input).WithBoundaryParams([]symbol.ID{table, sourceID})
	builder := NewBuilder(reg, Shape{Params: 2}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder, locals: make(map[symbol.ID]ValueTerm), expressions: make(map[factflow.ExprRef][]ValueTerm)}
	if err := bindBoundaryParamTerms(&ctx, Shape{Params: 2}); err != nil {
		t.Fatal(err)
	}
	var steps []rowStep
	ctx.rowSteps = &steps
	if err := (pathStorePlanHandler{kind: operationplan.PathAssignment}).Lower(ctx, point, nil); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || !builder.EffectArena().Valid(steps[0].effect, Shape{Params: 2}) {
		t.Fatalf("object path-store steps = %#v", steps)
	}
	cursor, err := NewBindingCursor(Shape{Params: 2}, []product.Value{product.Top(), product.Top()}, []pathdom.Path{pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1)})
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := builder.EffectArena().resolve(steps[0].effect, cursor, SpecializationContext{})
	if !ok {
		t.Fatal("object path-store did not resolve")
	}
	object := resolved.PathStore.Object
	if object.ListFloor != 1 || len(object.Heaps) != 2 || len(object.Heaps[0].Members) != 1 || len(object.Heaps[1].Members) != 2 || len(object.Entries) != 2 {
		t.Fatalf("resolved object payload = %#v", object)
	}
	if !object.Entries[0].Target.Equal(pathdom.NewPlaceholder(0).Field("value").IndexInt(1)) || !object.Entries[0].HasExpected {
		t.Fatalf("resolved first object entry = %#v", object.Entries[0])
	}
}
