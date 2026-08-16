package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestStaticIndexTermIsCanonicalRebasesAndMatchesSharedKernel(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	member := segment.Segment{Kind: segment.SegmentField, Name: "target_name"}
	term := callee.StaticIndexValue(callee.Root(Root{Kind: RootParam}), member)
	if term == 0 || term != callee.StaticIndexValue(callee.Root(Root{Kind: RootParam}), member) {
		t.Fatal("static index term is not hash-consed")
	}
	record := typetable.NewRecord().Field("target_name", typ.String).Build()
	owner := typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)
	owner = product.WithPresence(reg, owner, presence.Present())
	bound := caller.Constant(owner)
	bindings, _ := NewTermRootBindings(shape, Shape{}, []ValueTerm{bound}, nil)
	got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{term}})
	if err != nil || len(got.Values) != 1 || got.Values[0] != caller.StaticIndexValue(bound, member) {
		t.Fatalf("static index rebase = %#v/%v", got, err)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	projected, ok := caller.evalValue(got.Values[0], cursor, SpecializationContext{})
	key := typevalue.LiteralString(reg, "target_name")
	want, wantOK := enginesourcevalue.StaticIndexValue(reg, nil, owner, key)
	if !ok || !wantOK || !product.Equal(reg, projected, want) {
		t.Fatalf("term projection = %#v/%v, shared kernel %#v/%v", projected, ok, want, wantOK)
	}
}

func TestAdjustedDirectBindingAdmitsExactMemberProjection(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ref := factflow.ExprRef(71)
	alias := symbol.ID(22)
	plan := operationplan.New(graph, factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: pathdom.NewPath(alias, "route_entry").Field("target_name")},
	})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	container := builder.Arena().Constant(typevalue.FromType(reg, typ.NewArray(typetable.NewRecord().Field("target_name", typ.String).Build())))
	iterator := iteration.Iterator{Kind: iteration.IterateIndexed, Source: effect.ParamRef{Index: 0}}
	projection := builder.Arena().IteratorProjectionValue(iterator, 1, container)
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder, locals: map[symbol.ID]ValueTerm{alias: projection}}
	adjustedShape, _ := factflow.NewValueSourceShape(true, false, true, false)
	source, _ := factflow.NewExpressionValueSource(ref, 0, 0, 0, adjustedShape)
	value, path, err := exactDirectCallSourceBinding(ctx, source)
	if err != nil || value == 0 || path != 0 || builder.Arena().values[value].op != valueStaticIndex {
		t.Fatalf("adjusted member binding = %d/%d/%v", value, path, err)
	}

	malformed := source
	malformed.ResultIndex = factflow.NoValueSourceIndex
	callSource, _ := factflow.NewCallValueSource(ref, 0, 0, 0, 1, adjustedShape)
	varargSource, _ := factflow.NewVarargValueSource(ref, 0, 0, 0, adjustedShape)
	rootPathSource := source
	rootPathSource.ExprRef = ref + 1
	expandedSource := source
	expandedSource.Adjusted = false
	expandedSource.Expanded = true
	openSource := expandedSource
	openSource.OpenTail = true
	malformedExpression := source
	malformedExpression.HasExpr = false
	tests := []struct {
		name   string
		source factflow.ValueSource
	}{
		{name: "malformed-result-slot", source: malformed},
		{name: "call", source: callSource},
		{name: "vararg", source: varargSource},
		{name: "missing-expression-path", source: rootPathSource},
		{name: "expanded", source: expandedSource},
		{name: "open", source: openSource},
		{name: "malformed-expression", source: malformedExpression},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := exactDirectCallSourceBinding(ctx, test.source); err == nil {
				t.Fatal("adjusted non-member source accepted")
			}
		})
	}
}

func TestAdjustedDirectBindingAdmitsCertifiedRuntimeValidationScalar(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	innerRef, castRef := factflow.ExprRef(72), factflow.ExprRef(73)
	param := symbol.ID(23)
	scalarShape, _ := factflow.NewValueSourceShape(true, false, false, false)
	adjustedShape, _ := factflow.NewValueSourceShape(true, false, true, false)
	inner, _ := factflow.NewExpressionValueSource(innerRef, 0, 0, 0, scalarShape)
	cast, _ := factflow.NewExpressionValueSource(castRef, 0, 0, 0, adjustedShape)
	validatedType := typetable.NewRecord().Field("name", typ.String).Build()
	validated := typevalue.FromType(reg, validatedType)
	plan := operationplan.New(graph, factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			innerRef: pathdom.NewPath(param, "param"),
		},
		ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{
			castRef: factflow.NewExpressionRuntimeValidation(inner, validated),
		},
	})
	builder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{
		registry: reg,
		graph:    graph,
		plan:     plan,
		facts:    plan.Facts(),
		builder:  builder,
		locals: map[symbol.ID]ValueTerm{
			param: builder.Arena().Root(Root{Kind: RootParam}),
		},
		expressionRefinements: map[factflow.ExprRef]struct{}{castRef: {}},
	}
	value, path, err := exactDirectCallSourceBinding(ctx, cast)
	if err != nil || value == 0 || path != 0 || builder.Arena().values[value].op != valueExpressionRefinement {
		t.Fatalf("adjusted runtime validation binding = %d/%d/%v", value, path, err)
	}

	ctx.expressionRefinements = nil
	if _, _, err := exactDirectCallSourceBinding(ctx, cast); err == nil {
		t.Fatal("uncertified adjusted runtime validation accepted")
	}
}

func TestStaticIndexMalformedKeyFailsBeforeRebasePublication(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	root := callee.Root(Root{Kind: RootParam})
	term := callee.StaticIndexValue(root, segment.Segment{Kind: segment.SegmentField, Name: "field"})
	callee.values[term].args[1] = root // adversarial private-DAG corruption
	bound := caller.Root(Root{Kind: RootParam})
	bindings, _ := NewTermRootBindings(shape, shape, []ValueTerm{bound}, nil)
	before := len(caller.values)
	if got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{term}}); err == nil || len(got.Values) != 0 {
		t.Fatalf("malformed static key accepted: %#v/%v", got, err)
	}
	if len(caller.values) != before {
		t.Fatal("malformed static key partially published caller terms")
	}
}

func TestOperationPlanGenericAliasCarriesStaticIndexDirectBinding(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	iteratorCall := graph.AddNode(cfg.NodeCall)
	genericPoint := graph.AddNode(cfg.NodeAssign)
	aliasPoint := graph.AddNode(cfg.NodeAssign)
	graphSymbol, routeSymbol, routeEntrySymbol := symbol.ID(10), symbol.ID(20), symbol.ID(21)
	containerRef, memberRef := factflow.ExprRef(1), factflow.ExprRef(2)
	scalar, _ := factflow.NewValueSourceShape(true, false, false, false)
	adjusted, _ := factflow.NewValueSourceShape(true, false, true, false)
	containerSource, _ := factflow.NewExpressionValueSource(containerRef, 0, 0, 0, adjusted)
	memberSource, _ := factflow.NewExpressionValueSource(memberRef, 0, 0, 0, adjusted)
	aliasSource, _ := factflow.NewPathValueSource(pathdom.NewPath(routeSymbol, "route").Key(), 0, 0, 0, scalar)
	iteratorSite := factflow.NewCallSite(factflow.CallSiteConfig{Final: true, Expanded: true, ArgumentSources: []factflow.ValueSource{containerSource}})
	record := typetable.NewRecord().Field("target_name", typ.String).Build()
	op, ok := operationplan.NewGenericForOperation(1, routeSymbol, routeSymbol-1, []operationplan.GenericForSource{{
		Kind: operationplan.GenericForSourceCall, CallPoint: iteratorCall, HasCallPoint: true,
	}}, []typ.Type{typ.NewArray(record)})
	if !ok {
		t.Fatal("generic operation rejected")
	}
	op = op.WithIterator(iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed})
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{iteratorCall: iteratorSite},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{aliasPoint: factflow.NewRootAssignment(
			factflow.RootAssignmentLocalDeclaration, routeEntrySymbol, pathdom.NewPath(routeEntrySymbol, "route_entry"), aliasSource,
		)},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			containerRef: pathdom.NewPath(graphSymbol, "graph").Field("input_routes"),
			memberRef:    pathdom.NewPath(routeEntrySymbol, "route_entry").Field("target_name"),
		},
	}).WithBoundaryParams([]symbol.ID{graphSymbol}).WithExtensions([]operationplan.ExtensionInput{{
		Point: genericPoint, Kind: operationplan.BodyGenericFor, GenericFor: op,
	}})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx := prepared.base
	ctx.locals = make(map[symbol.ID]ValueTerm, len(prepared.base.locals))
	for id, term := range prepared.base.locals {
		ctx.locals[id] = term
	}
	ctx.genericBindings = make(map[symbol.ID]symbolicGenericBinding)
	var rootAssignment rootAssignmentTerm
	ctx.rootAssignment = &rootAssignment
	ctx.structuralEnvironment = true
	ctx.point = aliasPoint
	if _, err := lowerGenericForBinding(ctx, genericPoint, true); err != nil {
		t.Fatalf("generic binding: %v", err)
	}
	if err := (rootAssignmentPlanHandler{}).Lower(ctx, aliasPoint, nil); err != nil {
		t.Fatalf("alias assignment: %v", err)
	}
	value, path, err := exactDirectCallSourceBinding(ctx, memberSource)
	if err != nil || value == 0 || path != 0 {
		t.Fatalf("aliased member binding = %d/%d/%v", value, path, err)
	}
	node := prepared.builder.Arena().values[value]
	if node.op != valueDynamicTableRead || node.point != aliasPoint || len(node.args) != 2 || node.path == 0 || prepared.builder.Arena().values[node.args[0]].op != valueEnvironment {
		t.Fatalf("aliased member term is not the point-anchored post-N4 direct environment-table read: %#v", node)
	}
}
