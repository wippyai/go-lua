package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPreparedCompilerForwardsPreservedParameterThroughDirectCall(t *testing.T) {
	reg := standard.Registry()
	calleeShape := Shape{Params: 1}
	calleePlan := operationplan.New(cfg.New(), factflow.FactsInput{})
	certificate, err := CertifyPlan(calleePlan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	calleeBuilder := NewBuilder(reg, calleeShape, pathRefinementCapabilities(t), calleePlan)
	calleeRoot := Root{Kind: RootParam, Index: 0}
	callee, err := calleeBuilder.Build(certificate, []Row{{
		Guard:           calleeBuilder.Arena().True(),
		Ops:             []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: calleeBuilder.Arena().Constant(typevalue.LiteralString(reg, "done"))}},
		PathRefinements: []PathRefinementTerm{{Path: calleeBuilder.Arena().Path(calleeRoot), Value: calleeBuilder.Arena().Root(calleeRoot)}},
		Observations:    []ObservationTerm{{BodyOwner: testObservationBody(3), Kind: ObservationAssignment, Anchor: testObservationAnchor(ObservationAssignment, 1, 0), Guard: calleeBuilder.Arena().True(), Actual: calleeBuilder.Arena().Root(calleeRoot)}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	callee.observationComplete = true

	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	param, result := symbol.ID(8301), symbol.ID(8302)
	scalar, _ := factflow.NewValueSourceShape(true, false, false, false)
	argument, _ := factflow.NewPathValueSource(pathdom.NewPath(param, "p").Key(), 0, 0, 0, scalar)
	returned, _ := factflow.NewPathValueSource(pathdom.NewPath(result, "result").Key(), 0, 0, 0, scalar)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: call, HasPoint: true, Final: true, Expanded: true,
		ArgumentSources: []factflow.ValueSource{argument},
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, pathdom.NewPath(result, "result")),
		},
	})
	lowered := wir.NewBody("caller")
	start := lowered.Emit(wir.Instruction{Op: wir.OpCall})
	lowered.SetPointRange(call, start, start+1)
	lowered.AssignDebugPointOrdinals(graph)
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{call: site},
		Returns:   map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{returned})},
	}).WithObservationIdentity(testObservationBody(4), lowered).WithBoundaryParams([]symbol.ID{param})
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	calleeRef := CellRef{Function: 8303}
	catalog, err := NewDirectCallCatalog(graph.Size(), map[cfg.Point]DirectCallTarget{call: {Cell: calleeRef, Shape: calleeShape}})
	if err != nil {
		t.Fatal(err)
	}
	view := RelationView{values: map[CellRef]Relation{calleeRef: callee}, allowed: map[CellRef]struct{}{calleeRef: {}}}
	caller := prepared.EvaluateDirect(view, catalog)
	if caller.ContextualReason() != "" || caller.Widened() || caller.Rows() != 1 {
		t.Fatalf("forwarded chain relation = reason %q widened %v rows %d", caller.ContextualReason(), caller.Widened(), caller.Rows())
	}
	argumentValue := typevalue.LiteralString(reg, "boundary")
	cursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{argumentValue}, []pathdom.Path{pathdom.NewPlaceholder(0)})
	detailed, exact := caller.SpecializeDetailed(cursor, nil, SpecializationContext{})
	if !exact || len(detailed.Summary.NormalReturnFacts.PathRefinements) != 0 || len(detailed.PreservedParams) != 1 || detailed.PreservedParams[0] != 0 {
		t.Fatalf("forwarded chain specialization = %#v/%v, want canonical Summary plus preserved param 0", detailed, exact)
	}
	items := detailed.Observations.Items()
	if caller.ObservationCoverageComplete() || len(items) < 2 {
		t.Fatalf("direct observation projection complete=%v items=%#v, want evidence but fail-closed whole-owner coverage", caller.ObservationCoverageComplete(), items)
	}
	var assignmentSeen, callResultSeen bool
	for _, item := range items {
		assignmentSeen = assignmentSeen || item.Kind == ObservationAssignment && product.Equal(reg, item.Actual, argumentValue)
		callResultSeen = callResultSeen || item.Kind == ObservationCallResult && product.Equal(reg, item.Actual, typevalue.LiteralString(reg, "done"))
	}
	if !assignmentSeen || !callResultSeen {
		t.Fatalf("direct observation projection = %#v", items)
	}
}

func TestPlanCompilerEmitsCertifiedUnchangedParameterRoot(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	param := symbol.ID(8101)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	source, _ := factflow.NewPathValueSource(pathdom.NewPath(param, "value").Key(), 0, 0, 0, shape)
	plan := operationplan.New(graph, factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{
		ret: factflow.NewReturn([]factflow.ValueSource{source}),
	}}).WithBoundaryParams([]symbol.ID{param})
	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{Params: 1})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatal(reason)
	}
	argument := typevalue.LiteralString(reg, "unchanged")
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{argument}, []pathdom.Path{pathdom.NewPlaceholder(0)})
	if err != nil {
		t.Fatal(err)
	}
	detailed, exact := relation.SpecializeDetailed(cursor, nil, SpecializationContext{})
	if !exact || len(detailed.Summary.NormalReturnFacts.PathRefinements) != 0 || len(detailed.PreservedParams) != 1 || detailed.PreservedParams[0] != 0 {
		t.Fatalf("identity specialization = %#v/%v, want canonical Summary plus preserved param 0", detailed, exact)
	}
	if len(detailed.Summary.Returns) != 1 || !product.Equal(reg, detailed.Summary.Returns[0], argument) {
		t.Fatalf("identity return = %#v, want argument", detailed.Summary.Returns)
	}
}

func TestParamPreservationLedgerRejectsAliasMutationEscapeAndCall(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	param, local := symbol.ID(8201), symbol.ID(8202)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	paramSource, _ := factflow.NewPathValueSource(pathdom.NewPath(param, "p").Key(), 0, 0, 0, shape)
	plan := operationplan.New(graph, factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, local, pathdom.NewPath(local, "alias"), paramSource),
	}}).WithBoundaryParams([]symbol.ID{param})
	builder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), plan)
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder, locals: make(map[symbol.ID]ValueTerm)}
	if err := bindBoundaryParamTerms(&ctx, Shape{Params: 1}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		act  func(*paramPreservationLedger)
	}{
		{name: "alias", act: func(l *paramPreservationLedger) { l.observeFact(ctx, point, operationplan.RootAssignment) }},
		{name: "mutation", act: func(l *paramPreservationLedger) { l.observeFact(ctx, point, operationplan.DynamicIndexWrite) }},
		{name: "escape", act: func(l *paramPreservationLedger) {
			l.invalidateValueDependencies(builder.Arena(), builder.Arena().Root(Root{Kind: RootParam}))
		}},
		{name: "call", act: func(l *paramPreservationLedger) { l.observeFact(ctx, point, operationplan.CallSite) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger := newParamPreservationLedger(1)
			test.act(&ledger)
			row := SymbolicCFGRow{
				Guard: builder.Arena().True(), Values: ctx.locals, Output: emptyNormalReturnParams(1), paramPreserved: ledger,
			}
			if got := ledger.certifiedRefinements(builder.Arena(), builder.EffectArena(), Shape{Params: 1}, row, []symbol.ID{param}, nil); len(got) != 0 {
				t.Fatalf("uncertified %s emitted %#v", test.name, got)
			}
		})
	}

	t.Run("parameter-reassignment-target", func(t *testing.T) {
		literal, _ := factflow.NewIntegerLiteralValueSource(2, 0, 0, 0, shape)
		reassignPlan := operationplan.New(graph, factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, param, pathdom.NewPath(param, "p"), literal),
		}}).WithBoundaryParams([]symbol.ID{param})
		reassignBuilder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), reassignPlan)
		reassignCtx := planCompileContext{registry: reg, graph: graph, plan: reassignPlan, facts: reassignPlan.Facts(), builder: reassignBuilder, locals: make(map[symbol.ID]ValueTerm)}
		if err := bindBoundaryParamTerms(&reassignCtx, Shape{Params: 1}); err != nil {
			t.Fatal(err)
		}
		ledger := newParamPreservationLedger(1)
		ledger.observeFact(reassignCtx, point, operationplan.RootAssignment)
		if ledger.preserves(0) {
			t.Fatal("parameter target reassignment retained preservation proof")
		}
		relation := NewPlanCompiler().Compile(reg, graph, reassignPlan, Shape{Params: 1})
		if relation.ContextualReason() == "" || relation.Rows() != 0 {
			t.Fatalf("p = 2 relation did not fail closed: %q/%d rows", relation.ContextualReason(), relation.Rows())
		}
	})
}

func TestParamPreservationLedgerPreservesOnlyExactBoundaryStringMatchReceiver(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	param := symbol.ID(8251)

	check := func(t *testing.T, site factflow.CallSite, sig signature.Function, want bool) {
		t.Helper()
		op, ok := operationplan.NewSignatureCallOperation(sig)
		if !ok {
			t.Fatal("signature operation rejected")
		}
		plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{call: site}}).
			WithBoundaryParams([]symbol.ID{param}).
			WithBoundaryParamContracts([]product.Value{typevalue.String(reg)}).
			WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{call: op})
		ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), plan)}
		ledger := newParamPreservationLedger(1)
		ledger.observeFact(ctx, call, operationplan.CallSite)
		if got := ledger.preserves(0); got != want {
			t.Fatalf("preserves = %v, want %v", got, want)
		}
	}

	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	receiver := pathdom.NewPath(param, "id")
	receiverSource, _ := factflow.NewPathValueSource(receiver.Key(), 0, 0, 0, shape)
	pattern, _ := factflow.NewStringLiteralValueSource("^__", 0, 0, 0, shape)
	result := factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{})
	matchSite := factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: receiver.Field("match"), CalleeMemberAccess: true,
		ReceiverPath: receiver, HasReceiverPath: true, MethodPath: receiver.Field("match"), HasMethodPath: true, MethodName: "match",
		ReceiverSource: receiverSource, HasReceiverSource: true,
		ArgumentSources: []factflow.ValueSource{pattern}, ResultTargets: []factflow.CallResultTarget{result},
	})
	base, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("string.match")
	refined, exact := effectlowering.RefineStaticStringMethodSignature(reg, base, matchSite.View())
	if !exact {
		t.Fatal("exact match fixture did not refine")
	}
	check(t, matchSite, refined, true)

	effectful := refined.Clone()
	effectful.Effect = effect.Row{Tail: &effect.Var{Name: "e"}}
	check(t, matchSite, effectful, false)

	gsubSite := factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: receiver.Field("gsub"), CalleeMemberAccess: true,
		ReceiverPath: receiver, HasReceiverPath: true, MethodPath: receiver.Field("gsub"), HasMethodPath: true, MethodName: "gsub",
		ReceiverSource: receiverSource, HasReceiverSource: true, ResultTargets: []factflow.CallResultTarget{result},
	})
	check(t, gsubSite, signature.Function{Type: typ.Func().Returns(typ.String).Build()}, false)

	malformedMatch := factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: receiver.Field("match"), CalleeMemberAccess: true,
		ReceiverPath: receiver, HasReceiverPath: true, MethodPath: receiver.Field("match"), HasMethodPath: true, MethodName: "match",
		ReceiverSource: receiverSource, HasReceiverSource: true, ResultTargets: []factflow.CallResultTarget{result},
	})
	check(t, malformedMatch, refined, false)
}

func TestParamPreservationLedgerParticipatesInCloneEqualityAndHash(t *testing.T) {
	arena := NewArena(standard.Registry())
	row := SymbolicCFGRow{Guard: arena.True(), Values: map[symbol.ID]ValueTerm{}, paramPreserved: newParamPreservationLedger(2)}
	clone := cloneCFGRow(row)
	if !equalCFGRow(arena, row, clone) || exactWTOCFGRowHash(row) != exactWTOCFGRowHash(clone) {
		t.Fatal("cloned preservation proof changed row identity")
	}
	clone.paramPreserved.invalidate(1)
	if equalCFGRow(arena, row, clone) || exactWTOCFGRowHash(row) == exactWTOCFGRowHash(clone) {
		t.Fatal("preservation proof was omitted from row equality or fingerprint")
	}
	if !row.paramPreserved.preserves(1) {
		t.Fatal("clone invalidation mutated the source transaction")
	}
}

func emptyNormalReturnParams(count int) summary.Summary {
	return summary.Summary{NormalReturnParams: make([]product.Value, count)}
}
