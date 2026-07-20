package transformer

import (
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRelationProgramExecutesAllocationAsOneFormalTransaction(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), callPoint, false)
	graph.AddEdge(callPoint, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	ref := factflow.ExprRef(1)
	shape, _ := factflow.NewValueSourceShape(true, false, true, false)
	returned, _ := factflow.NewCallValueSource(ref, 0, 0, 0, callPoint, shape)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource, Point: callPoint, HasPoint: true,
		ExprRef: ref, HasExpr: true, Final: true, Adjusted: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{}),
		},
	})
	sig, ok := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	if !ok {
		t.Fatal("table.create signature")
	}
	call, ok := operationplan.NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("table.create call operation")
	}
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create allocation template")
	}
	allocation, ok := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 41, Template: template.Root, Ordinal: uint32(callPoint),
	}, template)
	if !ok {
		t.Fatal("table.create allocation operation")
	}

	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("relation-program-allocation-transaction"))
	bodyID := lexicalidentity.RootBody(namespace)
	w := wir.NewBody("allocation")
	start := w.Len()
	w.Emit(wir.Instruction{Op: wir.OpCall, Point: callPoint})
	w.SetPointRange(callPoint, start, w.Len())
	w.AssignDebugPointOrdinals(graph)
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{callPoint: site},
		Returns:   map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{returned})},
	}).WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{callPoint: call}).
		WithSignatureAllocations(map[cfg.Point]operationplan.SignatureAllocationOperation{callPoint: allocation}).
		WithBoundaryParams(nil).WithBoundaryCaptures(nil).WithBoundaryGlobals(nil)
	target, ok := operationplan.NewExternalCallSurfaceTarget(call)
	if !ok {
		t.Fatal("table.create call-surface target")
	}
	surface, err := operationplan.SealCallSurface(bodyID, graph.Size(), []cfg.Point{callPoint}, []operationplan.CallSurfaceSite{{Point: callPoint, Target: target}})
	if err != nil {
		t.Fatal(err)
	}
	plan = plan.WithObservationIdentity(bodyID, w, graph).WithCallSurface(surface)
	resolver := visibility.NewResolver(nil)
	paths := factapply.NewPathSemanticAuthority(resolver, nil, nil)
	keys := resolver.KeySpace()
	program, err := FreezeRelationProgram([]RelationProgramUnit{{
		Body: bodyID, Registry: reg, KeySpace: keys, Graph: graph, Plan: plan,
		Shape: Shape{}, PathSemantics: paths, Returns: factapply.NewReturnAuthority(paths, plan.Facts()), Domain: state.RegisteredProductDomain(reg), EntrySeedPlan: state.NewEntrySeedPlan(nil), InitialStatePlan: testInitialStatePlan(t, bodyID, graph),
	}}, testAcyclicCallTopology(t, bodyID))
	if err != nil {
		t.Fatal(err)
	}
	body := &program.bodies[0]
	if body.relation.code == nil || !body.relation.code.valid(body.relation.root) {
		t.Fatal("allocation test did not freeze the replacement RelationProgram")
	}
	effectCount := 0
	for ref := relationRootRef(1); int(ref) < len(body.relation.code.nodes); ref++ {
		for _, step := range body.relation.code.nodes[ref].steps {
			if step.kind == boundaryStepEffect && body.relation.effects.Kind(step.effect) == EffectAllocationTemplate {
				effectCount++
			}
		}
	}
	if effectCount != 1 {
		t.Fatalf("allocation prefix count = %d, want 1", effectCount)
	}
	if len(body.relation.arena.allocations) != 2 || len(body.relation.arena.allocations[1].templates) != 1 {
		t.Fatalf("sealed allocation inventory = %#v", body.relation.arena.allocations)
	}
	lexical := body.relation.arena.allocations[1].templates[0]
	actual, ok := body.rootAllocations.RebaseAllocation(lexical)
	if !ok || !identity.IsRootBoundaryAllocation(actual) {
		t.Fatalf("root boundary allocation = %#v/%v", actual, ok)
	}
	assertFormalAllocationTemplateDifferential(t, program)

	view, err := program.Solve(t.Context(), bodyID, state.Domain(reg).Bottom())
	if err != nil {
		t.Fatal(err)
	}
	var publication FormalLexicalBodyCoordinates
	foundBody := false
	for _, published := range view.LexicalBodies() {
		if published.Body == bodyID {
			publication, foundBody = published, true
			break
		}
	}
	if !foundBody {
		t.Fatal("formal solve has no allocation-body lexical publication")
	}
	callOutcome, exact := publication.CallOutcomes[callPoint]
	if !exact || len(callOutcome.Results) != 1 || callOutcome.Results[0].Index != 0 ||
		!callOutcome.PostReturnAuthority || callOutcome.SuspensionKnown || callOutcome.MaySuspend {
		t.Fatalf("allocation CallOutcome = %#v/%v, want exact authoritative non-suspension-unknown allocation", callOutcome, exact)
	}
	callResultID, resultExact := identityvalue.ExactID(reg, callOutcome.Results[0].Value)
	if !resultExact || callResultID != actual {
		t.Fatalf("allocation CallOutcome result = %#v/%v, want boundary identity %#v", callResultID, resultExact, actual)
	}
	callObject, objectExact := callOutcome.HeapTableObjects[actual]
	callPlacement, placementExact := callOutcome.Placements[actual]
	if !objectExact || !placementExact || len(callOutcome.HeapTableObjects) != 1 || len(callOutcome.Placements) != 1 ||
		!product.Equal(reg, callObject.Root(), callOutcome.Results[0].Value) || callPlacement == placement.Bottom {
		t.Fatalf("allocation CallOutcome object/placement = %#v/%v %#v/%v", callObject, objectExact, callPlacement, placementExact)
	}
	got, ok := publication.PlannedNodeOutputs[ret]
	if !ok || !publication.NodeOutputReachable[ret] {
		t.Fatal("allocation return node has no reachable formal publication")
	}
	returnID, exact := identityvalue.ExactID(reg, got.ReadReturnSlot(reg, 0))
	if !exact || returnID != actual {
		t.Fatalf("return allocation = %#v/%v, want boundary identity %#v", returnID, exact, actual)
	}
	object := got.ReadHeapTableObject(reg, actual)
	objectID, objectExact := identityvalue.ExactID(reg, object.Root())
	if !objectExact || objectID != actual || got.ReadPlacement(actual) == placement.Bottom {
		t.Fatalf("published object/placement diverged: object=%#v identity=%#v/%v placement=%v", object, objectID, objectExact, got.ReadPlacement(actual))
	}
	if !product.Equal(reg, object.Root(), got.ReadReturnSlot(reg, 0)) {
		t.Fatal("return and heap root do not share one published identity value")
	}
	factors, factorErr := body.productDomain.Decompose(got)
	if factorErr != nil {
		t.Fatal(factorErr)
	}
	for _, factor := range factors {
		hasTemplate, containsErr := body.productDomain.LaneContainsAllocationTemplate(factor)
		if containsErr != nil {
			t.Fatal(containsErr)
		}
		if hasTemplate {
			t.Fatalf("terminal state retained allocation syntax in lane %q", factor.Lane().ID())
		}
	}
}

// Signature allocations own a closed value/effect transaction. Provider
// operand exactness belongs only to external-call producers and therefore
// cannot participate in allocation admission, regardless of the operand
// source shape attached to the descriptive call site.
func TestSignatureAllocationLoweringDoesNotDemandExternalProviderOperands(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	unsupportedOperand := factflow.ValueSource{
		Kind: factflow.ValueSourceExpression, ExprRef: 99101, HasExpr: true,
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextStatement, Point: callPoint, HasPoint: true,
		ArgumentSources: []factflow.ValueSource{unsupportedOperand}, Final: true, Adjusted: true,
	})
	sig, ok := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	if !ok {
		t.Fatal("table.create signature")
	}
	call, ok := operationplan.NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("table.create call operation")
	}
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create allocation template")
	}
	allocation, ok := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 73, Template: template.Root, Ordinal: uint32(callPoint),
	}, template)
	if !ok {
		t.Fatal("table.create allocation operation")
	}
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{callPoint: site},
	}).WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{callPoint: call}).
		WithSignatureAllocations(map[cfg.Point]operationplan.SignatureAllocationOperation{callPoint: allocation})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	allocationTerm := builder.Arena().AllocationTemplate(allocation)
	effect, err := builder.EffectArena().AllocationTemplate(allocationTerm)
	if err != nil {
		t.Fatal(err)
	}
	steps := []rowStep(nil)
	output := structuralOutputContribution{}
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder,
		allocationEffects: map[cfg.Point]EffectTerm{callPoint: effect},
		rowSteps:          &steps, structuralOutput: &output,
	}
	if _, err := externalCallAccessTerms(ctx, callPoint); err == nil {
		t.Fatal("fixture provider operand unexpectedly became compiler-exact")
	}
	if err := (signatureCallPlanHandler{}).Lower(ctx, callPoint, nil); err != nil {
		t.Fatalf("allocation transaction was coupled to provider operands: %v", err)
	}
	if len(steps) != 1 || steps[0].kind != rowStepEffect || steps[0].effect != effect {
		t.Fatalf("allocation steps = %#v, want one canonical allocation effect %d", steps, effect)
	}
	if output.externalSealed || len(output.externalAccess) != 0 {
		t.Fatalf("allocation manufactured external producer access: %#v", output)
	}
}

func TestSignatureAllocationLoweringPublishesLocalResultWithSharedEffect(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeCall)
	target := symbol.ID(7401)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextAssignmentSource, Point: point, HasPoint: true,
		Final: true, Adjusted: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, pathdom.NewPath(target, "allocation")),
		},
	})
	sig, ok := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	if !ok {
		t.Fatal("table.create signature")
	}
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create allocation template")
	}
	op, ok := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 74, Template: template.Root, Ordinal: uint32(point),
	}, template)
	if !ok {
		t.Fatal("table.create allocation operation")
	}
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{point: site}}).
		WithSignatureAllocations(map[cfg.Point]operationplan.SignatureAllocationOperation{point: op})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	allocation := builder.Arena().AllocationTemplate(op)
	effect, err := builder.EffectArena().AllocationTemplate(allocation)
	if err != nil {
		t.Fatal(err)
	}
	steps := []rowStep(nil)
	locals := make(map[symbol.ID]ValueTerm)
	ctx := planCompileContext{
		registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder,
		allocationEffects: map[cfg.Point]EffectTerm{point: effect}, rowSteps: &steps, locals: locals,
	}
	if err := lowerSignatureAllocationTransaction(ctx, point); err != nil {
		t.Fatal(err)
	}
	want := builder.Arena().AllocationResultValue(allocation, template.ReturnIndex)
	if got := locals[target]; got != want || got == 0 {
		t.Fatalf("allocation destination = %d, want shared result term %d", got, want)
	}
	if len(steps) != 1 || steps[0].kind != rowStepEffect || steps[0].effect != effect {
		t.Fatalf("allocation steps = %#v, want one correlated effect %d", steps, effect)
	}
}

func TestFreezeRelationProgramRejectsAllocationWithLexicalCallOwnership(t *testing.T) {
	for _, test := range []struct {
		name     string
		residual bool
	}{
		{name: "exact lexical surface"},
		{name: "finite lexical candidates with residual", residual: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reg := standard.Registry()
			namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-lexical-owner-" + test.name))
			callerID := lexicalidentity.RootBody(namespace)
			targetID := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-lexical-target-" + test.name)))
			caller, callPoint := malformedLexicalAllocationUnit(t, reg, callerID, targetID, test.residual)
			target := emptyAllocationCollisionTargetUnit(t, reg, targetID)

			boundaries := []operationplan.CallTopologyBoundaryInput{{Body: callerID}, {Body: targetID}}
			var sites []operationplan.CallTopologySiteInput
			if test.residual {
				sites = []operationplan.CallTopologySiteInput{{
					Owner: callerID, Point: callPoint,
					Candidates: []operationplan.CallTopologyCandidate{{
						Identity: identity.ID{Kind: "lua.function", Site: "symbol", Index: 1}, Target: targetID,
					}},
				}}
			}
			topology, err := operationplan.SealCallTopology(
				[]lexicalidentity.StableLexicalBodyID{callerID, targetID}, sites, nil, boundaries,
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := FreezeRelationProgram([]RelationProgramUnit{caller, target}, topology); err == nil ||
				!strings.Contains(err.Error(), "allocation call") || !strings.Contains(err.Error(), "exact external signature surface") {
				t.Fatalf("FreezeRelationProgram error = %v, want allocation/external ownership rejection", err)
			}
		})
	}
}

func malformedLexicalAllocationUnit(
	t *testing.T,
	reg *axis.Registry,
	bodyID lexicalidentity.StableLexicalBodyID,
	targetID lexicalidentity.StableLexicalBodyID,
	residual bool,
) (RelationProgramUnit, cfg.Point) {
	t.Helper()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), callPoint, false)
	graph.AddEdge(callPoint, graph.Exit(), false)
	ref := factflow.ExprRef(1)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextStatement, Point: callPoint, HasPoint: true,
		ExprRef: ref, HasExpr: true, Final: true, Adjusted: true,
	})
	sig, ok := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	if !ok {
		t.Fatal("table.create signature")
	}
	signatureCall, ok := operationplan.NewSignatureCallOperation(sig)
	if !ok {
		t.Fatal("table.create call operation")
	}
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create allocation template")
	}
	allocation, ok := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 91, Template: template.Root, Ordinal: uint32(callPoint),
	}, template)
	if !ok {
		t.Fatal("table.create allocation operation")
	}
	code := wir.NewBody("allocation-lexical-collision")
	start := code.Len()
	code.Emit(wir.Instruction{Op: wir.OpCall, Point: callPoint})
	code.SetPointRange(callPoint, start, code.Len())
	code.AssignDebugPointOrdinals(graph)
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}}).
		WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{callPoint: signatureCall}).
		WithSignatureAllocations(map[cfg.Point]operationplan.SignatureAllocationOperation{callPoint: allocation}).
		WithBoundaryParams(nil).WithBoundaryCaptures(nil).WithBoundaryGlobals(nil)
	var surfaceTarget operationplan.CallSurfaceTarget
	if residual {
		surfaceTarget = operationplan.RejectedCallSurfaceTarget()
	} else {
		surfaceTarget, ok = operationplan.NewLexicalCallSurfaceTarget(targetID)
		if !ok {
			t.Fatal("lexical call target")
		}
	}
	surface, err := operationplan.SealCallSurface(bodyID, graph.Size(), []cfg.Point{callPoint}, []operationplan.CallSurfaceSite{{Point: callPoint, Target: surfaceTarget}})
	if err != nil {
		t.Fatal(err)
	}
	plan = plan.WithObservationIdentity(bodyID, code, graph).WithCallSurface(surface)
	resolver := visibility.NewResolver(nil)
	paths := factapply.NewPathSemanticAuthority(resolver, nil, nil)
	return RelationProgramUnit{
		Body: bodyID, Registry: reg, KeySpace: resolver.KeySpace(), Graph: graph, Plan: plan,
		Domain: state.RegisteredProductDomain(reg), PathSemantics: paths,
		Returns: factapply.NewReturnAuthority(paths, plan.Facts()), EntrySeedPlan: state.NewEntrySeedPlan(nil),
		InitialStatePlan: testInitialStatePlan(t, bodyID, graph),
	}, callPoint
}

func emptyAllocationCollisionTargetUnit(
	t *testing.T,
	reg *axis.Registry,
	bodyID lexicalidentity.StableLexicalBodyID,
) RelationProgramUnit {
	t.Helper()
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	code := wir.NewBody("allocation-lexical-target")
	code.AssignDebugPointOrdinals(graph)
	plan := operationplan.New(graph, factflow.FactsInput{}).
		WithBoundaryParams(nil).WithBoundaryCaptures(nil).WithBoundaryGlobals(nil)
	surface, err := operationplan.SealCallSurface(bodyID, graph.Size(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan = plan.WithObservationIdentity(bodyID, code, graph).WithCallSurface(surface)
	resolver := visibility.NewResolver(nil)
	paths := factapply.NewPathSemanticAuthority(resolver, nil, nil)
	return RelationProgramUnit{
		Body: bodyID, Registry: reg, KeySpace: resolver.KeySpace(), Graph: graph, Plan: plan,
		Domain: state.RegisteredProductDomain(reg), PathSemantics: paths,
		Returns: factapply.NewReturnAuthority(paths, plan.Facts()), EntrySeedPlan: state.NewEntrySeedPlan(nil),
		InitialStatePlan: testInitialStatePlan(t, bodyID, graph),
	}
}
