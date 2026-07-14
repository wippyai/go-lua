package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestGuardedObservationArtifactPreservesExpectedActualCorrelation(t *testing.T) {
	reg := standard.Registry()
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	builder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), plan)
	certificate, err := CertifyPlan(plan, DefaultSemanticCapabilityRegistry())
	if err != nil {
		t.Fatal(err)
	}
	arena := builder.Arena()
	actualAValue, actualBValue := typevalue.LiteralString(reg, "actual-a"), typevalue.LiteralNumber(reg, 42)
	wantAValue, wantBValue := typevalue.LiteralString(reg, "declared-a"), typevalue.LiteralNumber(reg, 99)
	actualA, actualB := arena.Constant(actualAValue), arena.Constant(actualBValue)
	wantA, wantB := arena.Constant(wantAValue), arena.Constant(wantBValue)
	relation, err := builder.Build(certificate, []Row{
		{Guard: arena.True(), Observations: []ObservationTerm{{BodyOwner: testObservationBody(1), Kind: ObservationCallResult, Anchor: testObservationAnchor(ObservationCallResult, 7, 0), Guard: arena.True(), Actual: actualA, Expected: wantA}}},
		{Guard: arena.True(), Observations: []ObservationTerm{{BodyOwner: testObservationBody(1), Kind: ObservationCallResult, Anchor: testObservationAnchor(ObservationCallResult, 7, 0), Guard: arena.True(), Actual: actualB, Expected: wantB}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralString(reg, "actual")}, nil)
	detailed, exact := relation.SpecializeDetailed(cursor, nil, SpecializationContext{})
	items := detailed.Observations.Items()
	if !exact || len(items) != 2 || !items[0].HasExpected || !items[1].HasExpected || items[0].Anchor != testObservationAnchor(ObservationCallResult, 7, 0) || items[1].Anchor != testObservationAnchor(ObservationCallResult, 7, 0) ||
		product.Equal(reg, items[0].Actual, items[1].Actual) || product.Equal(reg, items[0].Expected, items[1].Expected) {
		t.Fatalf("guarded observation artifact = %#v exact=%v", items, exact)
	}
	pairedA, pairedB := false, false
	for _, item := range items {
		pairedA = pairedA || product.Equal(reg, item.Actual, actualAValue) && product.Equal(reg, item.Expected, wantAValue)
		pairedB = pairedB || product.Equal(reg, item.Actual, actualBValue) && product.Equal(reg, item.Expected, wantBValue)
	}
	if !pairedA || !pairedB {
		t.Fatalf("expected/actual correlation torn: %#v", items)
	}
}

func TestDirectCompositionRetainsSameCalleeObservationAtTwoCallSites(t *testing.T) {
	reg := standard.Registry()
	calleePlan := operationplan.New(cfg.New(), factflow.FactsInput{}).WithBoundaryParams([]symbol.ID{1}).WithBoundaryParamContracts([]product.Value{product.Top()})
	calleeBuilder := NewBuilder(reg, Shape{Params: 1}, DefaultOutputCapabilityRegistry(), calleePlan)
	certificate, _ := CertifyPlan(calleePlan, DefaultSemanticCapabilityRegistry())
	param := calleeBuilder.Arena().Root(Root{Kind: RootParam})
	callee, err := calleeBuilder.Build(certificate, []Row{{Guard: calleeBuilder.Arena().True(), Ops: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: param}}, Observations: []ObservationTerm{{BodyOwner: testObservationBody(77), Kind: ObservationAssignment, Anchor: testObservationAnchor(ObservationAssignment, 1, 0), Guard: calleeBuilder.Arena().True(), Actual: param}}}})
	if err != nil {
		t.Fatal(err)
	}
	callerGraph := cfg.New()
	for callerGraph.Size() < 4 {
		callerGraph.AddNode(cfg.NodeCall)
	}
	for point := cfg.Point(1); int(point) < callerGraph.Size(); point++ {
		callerGraph.AddEdge(point-1, point, false)
	}
	lowered := wir.NewBody("caller")
	for _, point := range []cfg.Point{2, 3} {
		start := lowered.Emit(wir.Instruction{Op: wir.OpCall})
		lowered.SetPointRange(point, start, start+1)
	}
	lowered.AssignDebugPointOrdinals(callerGraph)
	callerPlan := operationplan.New(callerGraph, factflow.FactsInput{}).WithObservationIdentity(testObservationBody(88), lowered, callerGraph).WithBoundaryParams([]symbol.ID{11, 12}).WithBoundaryParamContracts([]product.Value{product.Top(), product.Top()})
	callerBuilder := NewBuilder(reg, Shape{Params: 2}, DefaultOutputCapabilityRegistry(), callerPlan)
	callerCertificate, _ := CertifyPlan(callerPlan, DefaultSemanticCapabilityRegistry())
	row := SymbolicCFGRow{Guard: callerBuilder.Arena().True(), Values: map[symbol.ID]ValueTerm{}}
	makeSite := func(point cfg.Point, result symbol.ID) factflow.CallSiteView {
		span := factflow.SourceSpan{StartLine: int(point), StartCol: 1, EndLine: int(point), EndCol: 4}
		site := factflow.NewCallSite(factflow.CallSiteConfig{Point: point, HasPoint: true, Final: true, Expanded: true, CallSpan: span, ArgumentSpans: []factflow.SourceSpan{span}, ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, pathdom.NewPath(result, "result"))}})
		view, _ := factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{point: site}}).CallSiteView(point)
		return view
	}
	for index, point := range []cfg.Point{2, 3} {
		root := Root{Kind: RootParam, Index: uint32(index)}
		rows, composeErr := ComposeDirectCallRows(callerBuilder, Shape{Params: 2}, row, callee, DirectCallBindings{Values: []ValueTerm{callerBuilder.Arena().Root(root)}, Paths: []PathTerm{callerBuilder.Arena().Path(root)}}, makeSite(point, symbol.ID(20+index)), 8)
		if composeErr != nil || len(rows) != 1 {
			t.Fatalf("compose %d = %#v/%v", point, rows, composeErr)
		}
		row = rows[0]
	}
	relation, err := callerBuilder.Build(callerCertificate, []Row{{Guard: row.Guard, Observations: row.Observations}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _ := NewBindingCursor(Shape{Params: 2}, []product.Value{typevalue.LiteralString(reg, "a"), typevalue.LiteralString(reg, "b")}, nil)
	detailed, exact := relation.SpecializeDetailed(cursor, nil, SpecializationContext{})
	items := detailed.Observations.Items()
	if !exact || len(items) != 4 {
		t.Fatalf("two-call observations = %#v exact=%v", items, exact)
	}
	calleeSeen := map[string]bool{}
	for _, item := range items {
		if item.Owner == testObservationBody(77) {
			value, _ := typevalue.StringLiteralOf(reg, item.Actual)
			calleeSeen[value] = true
		}
	}
	if !calleeSeen["a"] || !calleeSeen["b"] {
		t.Fatalf("callee invocation evidence collapsed: %#v", items)
	}
}

func TestObservationAdmissionFailsClosedOnUnknownKind(t *testing.T) {
	reg := standard.Registry()
	plan := operationplan.New(cfg.New(), factflow.FactsInput{})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	certificate, _ := CertifyPlan(plan, DefaultSemanticCapabilityRegistry())
	actual := builder.Arena().Constant(typevalue.LiteralString(reg, "actual"))
	if relation, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Observations: []ObservationTerm{{Kind: ObservationKind(99), Actual: actual}}}}); err == nil || relation.Rows() != 0 {
		t.Fatalf("unknown observation kind published: relation=%#v error=%v", relation, err)
	}
}
