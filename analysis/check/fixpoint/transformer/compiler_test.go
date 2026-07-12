package transformer

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPreparedPlanCompilerReusesArenaAndMatchesLegacy(t *testing.T) {
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
	compiler := NewPlanCompiler()
	prepared, err := compiler.Prepare(reg, graph, plan, Shape{})
	if err != nil {
		t.Fatal(err)
	}
	first, second := prepared.Evaluate(), prepared.Evaluate()
	if !EqualRelation(first, second) {
		t.Fatal("repeated prepared evaluation replaced its relation arena or rows")
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	preparedSummary, preparedOK := first.Specialize(cursor, nil, nil)
	want := summary.Normalize(reg, summary.Summary{Returns: []product.Value{typevalue.LiteralString(reg, "prepared")}})
	if !preparedOK || !summary.Equal(reg, preparedSummary, want) {
		t.Fatalf("prepared specialization differs\n got=%#v\nwant=%#v", preparedSummary, want)
	}
	equation, err := prepared.Equation(CellRef{Function: 101})
	if err != nil {
		t.Fatal(err)
	}
	cell, err := equation.Cell()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{cell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	frozen, ok := snapshot.Lookup(cell.Ref)
	if !ok || !EqualRelation(first, frozen) {
		t.Fatal("prepared RelationCell changed the persistent relation identity")
	}
}

func TestPreparedPlanCompilerDirectEquationComposesRowsAndRejectsRecursion(t *testing.T) {
	reg := standard.Registry()
	calleeShape := Shape{Params: 2}
	calleePlan := operationplan.New(cfg.New(), factflow.FactsInput{})
	calleeCertificate, err := CertifyPlan(calleePlan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	calleeBuilder := NewBuilder(reg, calleeShape, DefaultOutputCapabilityRegistry(), calleePlan)
	calleeParam := calleeBuilder.Arena().Root(Root{Kind: RootParam, Index: 1})
	left := calleeBuilder.Arena().Constant(typevalue.LiteralString(reg, "left"))
	right := calleeBuilder.Arena().Constant(typevalue.LiteralString(reg, "right"))
	nilValue := calleeBuilder.Arena().Constant(typevalue.Nil(reg))
	callee, err := calleeBuilder.Build(calleeCertificate, []Row{
		{Guard: calleeBuilder.Arena().Truthy(calleeParam), Ops: []Operation{
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: left},
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: nilValue},
		}},
		{Guard: calleeBuilder.Arena().Falsy(calleeParam), Ops: []Operation{
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: nilValue},
			{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 1, Value: right},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	returnPoint := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), callPoint, false)
	graph.AddEdge(callPoint, returnPoint, false)
	graph.AddEdge(returnPoint, graph.Exit(), false)
	callerParam0, callerParam1 := symbol.ID(201), symbol.ID(202)
	result0, result1 := symbol.ID(203), symbol.ID(204)
	scalarShape, _ := factflow.NewValueSourceShape(true, false, false, false)
	arg0, _ := factflow.NewPathValueSource(pathdom.NewPath(callerParam0, "a").Key(), 0, 0, 0, scalarShape)
	arg1, _ := factflow.NewPathValueSource(pathdom.NewPath(callerParam1, "b").Key(), 1, 1, 0, scalarShape)
	resultSource0, _ := factflow.NewPathValueSource(pathdom.NewPath(result0, "value").Key(), 0, 0, 0, scalarShape)
	resultSource1, _ := factflow.NewPathValueSource(pathdom.NewPath(result1, "err").Key(), 1, 1, 0, scalarShape)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: callPoint, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{arg0, arg1},
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result0, pathdom.NewPath(result0, "value")),
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, result1, pathdom.NewPath(result1, "err")),
		},
	})
	callerPlan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{callPoint: site},
		Returns:   map[cfg.Point]factflow.Return{returnPoint: factflow.NewReturn([]factflow.ValueSource{resultSource0, resultSource1})},
	}).WithBoundaryParams([]symbol.ID{callerParam0, callerParam1})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, callerPlan, Shape{Params: 2})
	if err != nil {
		t.Fatal(err)
	}
	calleeRef, callerRef := CellRef{Function: 301}, CellRef{Function: 302}
	catalog, err := NewDirectCallCatalog(graph.Size(), map[cfg.Point]DirectCallTarget{callPoint: {Cell: calleeRef, Shape: calleeShape}})
	if err != nil {
		t.Fatal(err)
	}
	view := RelationView{values: map[CellRef]Relation{calleeRef: callee}, allowed: map[CellRef]struct{}{calleeRef: {}}}
	firstDirect, secondDirect := prepared.EvaluateDirect(view, catalog), prepared.EvaluateDirect(view, catalog)
	if !EqualRelation(firstDirect, secondDirect) || firstDirect.authority == nil || !equalRelationOutputAuthority(firstDirect.authority, secondDirect.authority) {
		t.Fatal("repeated direct evaluation changed relation identity or output authority")
	}
	callerEquation, err := prepared.DirectEquation(callerRef, catalog)
	if err != nil {
		t.Fatal(err)
	}
	callerCell, _ := callerEquation.Cell()
	calleeCell := RelationCell{Ref: calleeRef, Arena: callee.arena, Shape: callee.shape, Equation: func(context.Context, RelationView) (Relation, error) { return callee, nil }}
	snapshot, err := SolveRelationCells(context.Background(), []RelationCell{callerCell, calleeCell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	caller, ok := snapshot.Lookup(callerRef)
	if !ok || caller.ContextualReason() != "" || caller.Widened() || caller.Rows() != 2 {
		t.Fatalf("direct caller relation = %#v/%v", caller, ok)
	}
	for _, test := range []struct {
		key  product.Value
		want summary.Summary
	}{
		{key: typevalue.LiteralBool(reg, true), want: summary.Summary{Returns: []product.Value{typevalue.LiteralString(reg, "left"), typevalue.Nil(reg)}}},
		{key: typevalue.LiteralBool(reg, false), want: summary.Summary{Returns: []product.Value{typevalue.Nil(reg), typevalue.LiteralString(reg, "right")}}},
	} {
		cursor, _ := NewBindingCursor(Shape{Params: 2}, []product.Value{product.Top(), test.key}, nil)
		got, exact := caller.Specialize(cursor, nil, nil)
		if !exact || len(got.Returns) != 2 || !product.Equal(reg, got.Returns[0], test.want.Returns[0]) || !product.Equal(reg, got.Returns[1], test.want.Returns[1]) || len(got.ReturnConditionSlotRefinements) == 0 || len(got.ReturnPresenceRelations) == 0 {
			t.Fatalf("direct caller specialization exact=%v\n got=%#v\nwant=%#v", exact, got, test.want)
		}
	}
	selfCatalog, _ := NewDirectCallCatalog(graph.Size(), map[cfg.Point]DirectCallTarget{callPoint: {Cell: callerRef, Shape: calleeShape}})
	if _, err := prepared.DirectEquation(callerRef, selfCatalog); err == nil {
		t.Fatal("recursive direct equation was not rejected")
	}
	leftRef, rightRef := CellRef{Function: 401}, CellRef{Function: 402}
	leftCatalog, _ := NewDirectCallCatalog(graph.Size(), map[cfg.Point]DirectCallTarget{callPoint: {Cell: rightRef, Shape: calleeShape}})
	rightCatalog, _ := NewDirectCallCatalog(graph.Size(), map[cfg.Point]DirectCallTarget{callPoint: {Cell: leftRef, Shape: calleeShape}})
	leftEquation, _ := prepared.DirectEquation(leftRef, leftCatalog)
	rightEquation, _ := prepared.DirectEquation(rightRef, rightCatalog)
	leftCell, _ := leftEquation.Cell()
	rightCell, _ := rightEquation.Cell()
	cycle, err := SolveRelationCells(context.Background(), []RelationCell{leftCell, rightCell}, RelationSolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	leftRelation, _ := cycle.Lookup(leftRef)
	rightRelation, _ := cycle.Lookup(rightRef)
	if leftRelation.ContextualReason() == "" || rightRelation.ContextualReason() == "" {
		t.Fatal("mutually recursive direct equations converged to under-approximate Bottom")
	}
	receiverOnly := factflow.NewCallSite(factflow.CallSiteConfig{
		ReceiverPath: pathdom.NewPath(callerParam0, "a"), HasReceiverPath: true,
		Final: true, Expanded: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result0, pathdom.NewPath(result0, "value")),
		},
	}).View()
	if binding, err := exactDirectCallBindings(prepared.base, Shape{Params: 1}, receiverOnly); err != nil || len(binding.Values) != 1 || len(binding.Paths) != 1 {
		t.Fatalf("canonical boundary receiver path binding = %#v/%v", binding, err)
	}
	adjustedArg := arg0
	adjustedArg.Adjusted = true
	adjustedSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Final: true, Expanded: true, ArgumentSources: []factflow.ValueSource{adjustedArg},
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result0, pathdom.NewPath(result0, "value")),
		},
	}).View()
	if _, err := exactDirectCallBindings(prepared.base, Shape{Params: 1}, adjustedSite); err == nil {
		t.Fatal("adjusted direct argument was silently treated as unadjusted")
	}
}

func TestExactDirectCallBindingAllowsUnusedMissingPathAndRejectsPathUse(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ref := factflow.ExprRef(91)
	value := typevalue.LiteralString(reg, "value-only")
	plan := operationplan.New(graph, factflow.FactsInput{
		ExpressionValues: map[factflow.ExprRef]product.Value{ref: value},
	})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{})
	if err != nil {
		t.Fatal(err)
	}
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	source, _ := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
	bound, path, err := exactDirectCallSourceBinding(prepared.base, source)
	if err != nil || bound == 0 || path != 0 {
		t.Fatalf("value-only binding = %d/%d/%v, want value and optional zero path", bound, path, err)
	}

	callee, caller := NewArena(reg), prepared.builder.Arena()
	calleeShape := Shape{Params: 1}
	bindings, err := NewTermRootBindings(calleeShape, Shape{}, []ValueTerm{bound}, []PathTerm{path})
	if err != nil {
		t.Fatal(err)
	}
	root := Root{Kind: RootParam, Index: 0}
	if _, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{callee.Root(root)}}); err != nil {
		t.Fatalf("unused missing path rejected value-only DAG: %v", err)
	}
	if got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Paths: []PathTerm{callee.Path(root)}}); err == nil || !reflect.DeepEqual(got, TermRebaseOutput{}) {
		t.Fatalf("used missing path did not fail atomically: %#v/%v", got, err)
	}
}

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
	wantInt := typevalue.LiteralInt(reg, 42)
	want := summary.Normalize(reg, summary.Summary{
		Returns: []product.Value{wantInt, wantString},
		ReturnConditionSlotRefinements: []summary.ReturnConditionSlotRefinement{
			{ReturnIndex: 0, ReturnValue: true, TargetIndex: 1, Value: wantString},
			{ReturnIndex: 1, ReturnValue: true, TargetIndex: 0, Value: wantInt},
		},
		ReturnPresenceRelations: []summary.ReturnPresenceRelation{
			{TriggerIndex: 0, TriggerPresence: presence.Present(), TargetIndex: 1, TargetPresence: presence.Present()},
			{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Present()},
		},
	})
	if !summary.Equal(reg, got, want) {
		t.Fatalf("specialized Summary differs\n got=%#v\nwant=%#v", got, want)
	}
}

func TestPlanCompilerReturnCorrelationUsesRawTriggerBeforeDeclaredContract(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	param := symbol.ID(7)
	paramPath := pathdom.NewPath(param, "value")
	rawTop, _ := factflow.NewPathValueSource(paramPath.Key(), 0, 0, 0, shape)
	second, _ := factflow.NewStringLiteralValueSource("ok", 1, 1, 0, shape)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	plan := operationplan.New(graph, factflow.FactsInput{
		Returns: map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{rawTop, second})},
	}).WithBoundaryParams([]symbol.ID{param}).WithBoundaryReturns([]product.Value{declared, declared})
	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{Params: 1})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatal(reason)
	}
	cursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{product.Top()}, []pathdom.Path{pathdom.NewPlaceholder(0)})
	got, ok := relation.Specialize(cursor, nil, nil)
	if !ok {
		t.Fatal("annotated multi-return relation did not specialize")
	}
	for _, refinement := range got.ReturnConditionSlotRefinements {
		if refinement.ReturnIndex == 0 {
			t.Fatalf("declared truthy contract invented raw-trigger condition: %#v", refinement)
		}
	}
}

func TestPlanCompilerScalarLocalAssignmentFeedsReturnExactly(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	literal, ok := factflow.NewIntegerLiteralValueSource(42, 0, 0, 0, shape)
	if !ok {
		t.Fatal("literal source rejected")
	}
	sym := symbol.ID(7)
	localPath := pathdom.Path{Root: "answer", Symbol: sym}
	read, ok := factflow.NewPathValueSource(localPath.Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("path source rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, sym, localPath, literal),
		},
		Returns: map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{read})},
	})

	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("assignment relation contextual: %s", reason)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	got, exact := relation.Specialize(cursor, nil, nil)
	want := summary.Normalize(reg, summary.Summary{Returns: []product.Value{typevalue.LiteralInt(reg, 42)}})
	if !exact || !summary.Equal(reg, got, want) {
		t.Fatalf("assignment relation exact=%v\n got=%#v\nwant=%#v", exact, got, want)
	}
}

func TestPlanCompilerRootAssignmentNarrowAdmissionFailsClosed(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	literal, _ := factflow.NewIntegerLiteralValueSource(42, 0, 0, 0, shape)
	sym := symbol.ID(7)
	root := pathdom.Path{Root: "answer", Symbol: sym}
	cases := []struct {
		name string
		fact factflow.RootAssignment
		want string
	}{
		{
			name: "descendant target",
			fact: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, sym,
				pathdom.Path{Root: "answer", Symbol: sym, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "x"}}}, literal),
			want: "canonical root symbol",
		},
		{
			name: "declared overlay",
			fact: factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentLocalDeclaration, sym, root, literal, typevalue.LiteralInt(reg, 42)),
			want: "contracts and overlays",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			graph := cfg.New()
			assign := graph.AddNode(cfg.NodeAssign)
			graph.AddEdge(graph.Entry(), assign, false)
			graph.AddEdge(assign, graph.Exit(), false)
			plan := operationplan.New(graph, factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: tc.fact}})
			relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
			if reason := relation.ContextualReason(); !strings.Contains(reason, tc.want) || relation.Rows() != 0 {
				t.Fatalf("contextual relation reason/rows = %q/%d", reason, relation.Rows())
			}
		})
	}
}

func TestPlanCompilerUnsupportedFamiliesFailAsOneContextualRelation(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	plan := operationplan.New(graph, factflow.FactsInput{
		DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{point: {}},
		PathAssignments:    map[cfg.Point]factflow.PathAssignment{point: {}},
		CallSites:          map[cfg.Point]factflow.CallSite{point: {}},
	})

	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	reason := relation.ContextualReason()
	const wantReason = "compiler: contextual operations: PathAssignments"
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
