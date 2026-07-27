package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// TestRelationProgramDirectCallPreservesConditionedPhi is the production-cut
// regression for whole-environment conditioning.  The helper is equivalent to
//
//	local function helper(x)
//	  local y
//	  if x then y = x else y = "s" end
//	  return y
//	end
//
// For x = nil|false|"t", both arms return a truthy value.  Closing the two
// environments as an unguarded scalar join instead loses the true-edge fact
// about x and widens the caller's result back to a value that may be falsy.
func TestRelationProgramDirectCallPreservesConditionedPhi(t *testing.T) {
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("relation-conditioned-phi-direct-call"))
	callerID := lexicalidentity.RootBody(namespace)
	helperID := lexicalidentity.FunctionBody(namespace, 1)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("scalar source shape rejected")
	}
	pathSource := func(id symbol.ID, name string) factflow.ValueSource {
		t.Helper()
		source, exact := factflow.NewPathValueSource(pathdom.NewPath(id, name).Key(), 0, 0, 0, shape)
		if !exact {
			t.Fatalf("path source %s rejected", name)
		}
		return source
	}

	helperGraph := cfg.New()
	branch := helperGraph.AddNode(cfg.NodeBranch)
	trueAssign := helperGraph.AddNode(cfg.NodeAssign)
	falseAssign := helperGraph.AddNode(cfg.NodeAssign)
	helperReturn := helperGraph.AddNode(cfg.NodeReturn)
	helperGraph.AddEdge(helperGraph.Entry(), branch, false)
	helperGraph.AddEdge(branch, trueAssign, true)
	helperGraph.AddEdge(branch, falseAssign, false)
	helperGraph.AddEdge(trueAssign, helperReturn, false)
	helperGraph.AddEdge(falseAssign, helperReturn, false)
	helperGraph.AddEdge(helperReturn, helperGraph.Exit(), false)
	helperParam, helperLocal := symbol.ID(4101), symbol.ID(4102)
	helperParamPath := pathdom.NewPath(helperParam, "x")
	condition, ok := factflow.NewBranchCondition(pathSource(helperParam, "x"), true)
	if !ok {
		t.Fatal("helper branch condition rejected")
	}
	stringSource, ok := factflow.NewStringLiteralValueSource("s", 0, 0, 0, shape)
	if !ok {
		t.Fatal("helper false-arm literal rejected")
	}
	helperFacts := factflow.FactsInput{
		BranchConditionSources: map[cfg.Point]factflow.BranchCondition{branch: condition},
		BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
			branch: factflow.NewBranchPathEvidenceSet(
				factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(helperParamPath, true),
			),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			trueAssign:  factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, helperLocal, pathdom.NewPath(helperLocal, "y"), pathSource(helperParam, "x")),
			falseAssign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, helperLocal, pathdom.NewPath(helperLocal, "y"), stringSource),
		},
		Returns: map[cfg.Point]factflow.Return{
			helperReturn: factflow.NewReturn([]factflow.ValueSource{pathSource(helperLocal, "y")}),
		},
	}
	helperVisibility := visibility.NewBuilder()
	for _, point := range []cfg.Point{branch, trueAssign, falseAssign, helperReturn} {
		helperVisibility.Define(point, helperParam, "x")
		helperVisibility.Define(point, helperLocal, "y")
	}
	helperResolver := visibility.NewResolver(helperVisibility.Build())
	helperWIR := wir.NewBody("helper")
	helperWIR.AssignDebugPointOrdinals(helperGraph)
	helperPlan := operationplan.New(helperGraph, helperFacts).
		WithBoundaryParams([]symbol.ID{helperParam}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	helperSurface, err := operationplan.SealCallSurface(helperID, helperGraph.Size(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	helperPlan = helperPlan.WithCallSurface(helperSurface)
	helperPaths := factapply.NewPathSemanticAuthority(helperResolver, nil, nil)

	callerGraph := cfg.New()
	call := callerGraph.AddNode(cfg.NodeCall)
	callerReturn := callerGraph.AddNode(cfg.NodeReturn)
	callerGraph.AddEdge(callerGraph.Entry(), call, false)
	callerGraph.AddEdge(call, callerReturn, false)
	callerGraph.AddEdge(callerReturn, callerGraph.Exit(), false)
	callerParam, callerResult := symbol.ID(4201), symbol.ID(4202)
	callSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextAssignmentSource,
		Point:   call, HasPoint: true, Final: true, Adjusted: true,
		ArgumentSources: []factflow.ValueSource{pathSource(callerParam, "input")},
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, callerResult, pathdom.NewPath(callerResult, "result")),
		},
	})
	callerFacts := factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{call: callSite},
		Returns: map[cfg.Point]factflow.Return{
			callerReturn: factflow.NewReturn([]factflow.ValueSource{pathSource(callerResult, "result")}),
		},
	}
	callerVisibility := visibility.NewBuilder()
	callerVisibility.Define(call, callerParam, "input")
	callerVisibility.Define(callerReturn, callerResult, "result")
	callerResolver := visibility.NewResolver(callerVisibility.Build())
	callerWIR := wir.NewBody("caller")
	start := callerWIR.Len()
	callerWIR.Emit(wir.Instruction{Op: wir.OpCall, Point: call})
	callerWIR.SetPointRange(call, start, callerWIR.Len())
	callerWIR.AssignDebugPointOrdinals(callerGraph)
	callerPlan := operationplan.New(callerGraph, callerFacts).
		WithBoundaryParams([]symbol.ID{callerParam}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	target, ok := operationplan.NewLexicalCallSurfaceTarget(helperID)
	if !ok {
		t.Fatal("helper call target rejected")
	}
	callerSurface, err := operationplan.SealCallSurface(callerID, callerGraph.Size(), []cfg.Point{call}, []operationplan.CallSurfaceSite{{Point: call, Target: target}})
	if err != nil {
		t.Fatal(err)
	}
	callerPlan = callerPlan.WithCallSurface(callerSurface)
	callerPaths := factapply.NewPathSemanticAuthority(callerResolver, nil, nil)

	program, err := FreezeRelationProgram([]RelationProgramUnit{
		{
			Body: callerID, Registry: reg, KeySpace: callerResolver.KeySpace(), Graph: callerGraph, Plan: callerPlan,
			Shape: Shape{Params: 1}, PathSemantics: callerPaths, Returns: factapply.NewReturnAuthority(callerPaths, callerPlan.Facts()),
			Domain: state.RegisteredProductDomain(reg), EntrySeedPlan: state.NewEntrySeedPlan(nil), InitialStatePlan: testInitialStatePlan(t, callerID, callerGraph),
		},
		{
			Body: helperID, Registry: reg, KeySpace: helperResolver.KeySpace(), Graph: helperGraph, Plan: helperPlan,
			Shape: Shape{Params: 1}, PathSemantics: helperPaths,
			RootAssignments: factapply.NewRootAssignmentAuthority(helperPaths, helperPlan.Facts(), nil, state.RegisteredProductDomain(reg)),
			Returns:         factapply.NewReturnAuthority(helperPaths, helperPlan.Facts()),
			Domain:          state.RegisteredProductDomain(reg), EntrySeedPlan: state.NewEntrySeedPlan(nil), InitialStatePlan: testInitialStatePlan(t, helperID, helperGraph),
		},
	}, testAcyclicCallTopology(t, callerID, helperID))
	if err != nil {
		t.Fatal(err)
	}
	callerVariable, ok := program.Variable(callerID)
	if !ok {
		t.Fatal("frozen program has no caller relation")
	}
	input := product.Join(reg,
		typevalue.Nil(reg),
		product.Join(reg, typevalue.LiteralBool(reg, false), typevalue.LiteralString(reg, "t")),
	)
	callerBody := &program.bodies[relationVar(callerVariable)-1]
	entry := callerBody.domain.Bottom().WriteValue(reg, callerBody.roots.roots[0].slot, input)
	view, err := program.Solve(t.Context(), callerID, entry)
	if err != nil {
		t.Fatal(err)
	}
	var callerPublication FormalLexicalBodyCoordinates
	foundCaller := false
	for _, published := range view.LexicalBodies() {
		if published.Body == callerID {
			callerPublication, foundCaller = published, true
			break
		}
	}
	if !foundCaller {
		t.Fatal("formal solve has no caller lexical publication")
	}
	bakedOutcome, bakedExact := callerPublication.CallOutcomes[call]
	if !bakedExact {
		t.Fatal("entry-baked solve has no direct-call outcome")
	}
	// The symbolic WTO product retains the caller's entry-dependent Values
	// until specialization.  Detaching the Apply observation is therefore a
	// completed-relation operation: its DTO must exactly match the normal
	// entry-baked publication after that one specialization pass.
	symbolic, err := executeFormalRelation(t.Context(), program)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := freezeFormalRootEntrySeed(program, callerID, entry)
	if err != nil {
		t.Fatal(err)
	}
	substitution, err := newFormalRootEntrySubstitution(seed)
	if err != nil {
		t.Fatal(err)
	}
	specialized, err := substitution.specializeStabilized(t.Context(), symbolic)
	if err != nil {
		t.Fatal(err)
	}
	specializedPublication, err := specialized.Publication(callerID)
	if err != nil {
		t.Fatal(err)
	}
	specializedOutcome, specializedExact, err := specializedPublication.CallOutcome(t.Context(), call)
	if err != nil || !specializedExact || !callpayload.CallOutcomeRepresentationEqual(bakedOutcome, specializedOutcome) {
		t.Fatalf("specialized direct-call outcome = exact:%t equal:%t err:%v", specializedExact, callpayload.CallOutcomeRepresentationEqual(bakedOutcome, specializedOutcome), err)
	}
	output, ok := callerPublication.PlannedNodeOutputs[callerReturn]
	if !ok || !callerPublication.NodeOutputReachable[callerReturn] {
		t.Fatal("caller return node has no reachable formal publication")
	}
	got := output.ReadValue(reg, statekey.ReturnSlot(0))
	if valuerefine.CanBeFalsy(reg, got) {
		t.Fatalf("conditioned helper result may be nil/false: %#v; want truthy-arm x joined only with false-arm string", got)
	}
}
