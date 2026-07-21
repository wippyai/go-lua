package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalRootAssignmentExecutesCanonicalSparseN4(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	const target, marker = symbol.ID(1701), symbol.ID(1702)
	source, ok := factflow.NewStringLiteralValueSource("formal-n4", 0, 0, 0, factflow.ValueSourceShape{})
	if !ok {
		t.Fatal("literal source")
	}
	facts := factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "target"), source),
	}}
	builder := visibility.NewBuilder()
	for _, point := range cfg.RPOReadOnly(graph) {
		builder.Define(point, target, "target")
		builder.Define(point, marker, "marker")
	}
	old := typevalue.LiteralString(reg, "old")
	untouched := typevalue.LiteralBool(reg, false)
	program, bodyID := freezeFormalRootAssignmentTestProgram(t, reg, graph, facts, visibility.NewResolver(builder.Build()), []symbol.ID{target, marker})
	entry := state.Domain(reg).Bottom().
		WriteValue(reg, statekey.SymbolValue(target), old).
		WriteValue(reg, statekey.SymbolValue(marker), untouched)
	view, err := program.Solve(t.Context(), bodyID, entry)
	if err != nil {
		t.Fatal(err)
	}
	var equation formalRelationEquation
	found := false
	for _, candidate := range program.formalTemplate.equations {
		operator, present := candidate.terminalOperator()
		if present && operator.stepCapability == formalRelationStepCapabilityRootAssignment {
			equation, found = candidate, true
			equation.Operator = operator
			break
		}
	}
	if !found || equation.Operator.rootAssignment == nil {
		t.Fatal("formal RootAssignment capability was not admitted")
	}
	plan := equation.Operator.rootAssignment
	span, ok := program.formalFibers.span(equation.Cell.cell.Variable)
	if !ok {
		t.Fatal("formal RootAssignment span")
	}
	sparseWidth := len(plan.currentOrdinals) + len(plan.pointOrdinals) + len(plan.demands)
	fullWidth := 2*span.count + len(plan.demands)
	if sparseWidth >= fullWidth {
		t.Fatalf("RootAssignment sparse correlation width=%d full=%d", sparseWidth, fullWidth)
	}
	var publication FormalLexicalBodyCoordinates
	foundBody := false
	for _, candidate := range view.LexicalBodies() {
		if candidate.Body == bodyID {
			publication, foundBody = candidate, true
			break
		}
	}
	if !foundBody {
		t.Fatal("formal RootAssignment solve has no lexical publication")
	}
	output, ok := publication.PlannedNodeOutputs[ret]
	if !ok || !publication.NodeOutputReachable[ret] {
		t.Fatal("formal RootAssignment return has no reachable publication")
	}
	want := typevalue.LiteralString(reg, "formal-n4")
	if got := output.ReadValue(reg, statekey.SymbolValue(target)); !product.Equal(reg, got, want) {
		t.Fatalf("formal RootAssignment target=%#v want=%#v", got, want)
	}
	if got := output.ReadValue(reg, statekey.SymbolValue(marker)); !product.Equal(reg, got, untouched) {
		t.Fatalf("formal RootAssignment changed unrelated Values slot: %#v", got)
	}
}

func TestFormalRootAssignmentOmitsPersistentUnchangedLane(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	lane, ok := domain.ProductLane(state.LaneNumFloors)
	if !ok {
		t.Fatal("num-floors lane")
	}
	original, err := domain.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	factors := formalRootAssignmentFactors{
		bindings: []formalRootAssignmentLaneBinding{{lane: lane}},
		values:   []state.LaneFactor{original},
		original: []state.LaneFactor{original},
	}
	got, changed, err := factors.changed(domain, lane)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("persistent operand identity was marked changed")
	}
	if same, err := domain.LaneSame(got, original); err != nil || !same {
		t.Fatalf("unchanged factor identity same=%t err=%v", same, err)
	}
}

// freezeFormalRootAssignmentTestProgram freezes the same immutable unit used
// by production: Plan -> RelationProgram -> one formal WTO equation system.
// It intentionally exposes no typed-semantic or concrete scheduler facade.
func freezeFormalRootAssignmentTestProgram(
	t *testing.T,
	reg *axis.Registry,
	graph cfg.Graph,
	facts factflow.FactsInput,
	resolver *visibility.Resolver,
	params []symbol.ID,
) (*RelationProgram, lexicalidentity.StableLexicalBodyID) {
	t.Helper()
	if reg == nil || resolver == nil {
		t.Fatal("formal RootAssignment fixture has no registry/resolver")
	}
	bodyID := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	wirBody := wir.NewBody("formal-root-assignment")
	wirBody.AssignDebugPointOrdinals(graph)
	plan := operationplan.New(graph, facts).
		WithBoundaryParams(params).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	surface, err := operationplan.SealCallSurface(bodyID, graph.Size(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan = plan.WithCallSurface(surface)
	domain := state.RegisteredProductDomain(reg)
	paths := factapply.NewPathSemanticAuthority(resolver, nil, nil)
	program, err := FreezeRelationProgram([]RelationProgramUnit{{
		Body: bodyID, Registry: reg, KeySpace: resolver.KeySpace(), Graph: graph, Plan: plan,
		Shape: Shape{Params: uint32(len(params))}, Domain: domain, PathSemantics: paths,
		RootAssignments:  factapply.NewRootAssignmentAuthority(paths, plan.Facts(), nil, domain),
		Returns:          factapply.NewReturnAuthority(paths, plan.Facts()),
		EntrySeedPlan:    state.NewEntrySeedPlan(nil),
		InitialStatePlan: testInitialStatePlan(t, bodyID, graph),
	}}, testAcyclicCallTopology(t, bodyID))
	if err != nil {
		t.Fatal(err)
	}
	return program, bodyID
}
