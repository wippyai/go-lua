package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFreezeRelationProgramOwnsOneImmutableFormalTemplate(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("relation-program-formal-template-freeze"))
	body := lexicalidentity.RootBody(namespace)
	unit := formalTemplateFreezeUnit(t, body)
	topology := testAcyclicCallTopology(t, body)

	program, err := FreezeRelationProgram([]RelationProgramUnit{unit}, topology)
	if err != nil {
		t.Fatal(err)
	}
	assertFormalTemplateOwnedByProgram(t, program)
	if !program.relationDependencyFreeze.validFor(program) ||
		program.relationDependencyFreeze.version != relationDependencyFreezeResultVersion ||
		len(program.relationDependencyFreeze.tier2.syntax) != len(program.syntax) ||
		len(program.relationDependencyFreeze.tier2.links) != len(program.links) {
		t.Fatal("frozen relation program has no sealed full-result-v1 dependency product")
	}
	for index := range program.syntax {
		syntax, link := program.relationDependencyFreeze.tier2.syntax[index], program.relationDependencyFreeze.tier2.links[index]
		if syntax.body != program.syntax[index].body || syntax.variable != program.syntax[index].variable ||
			syntax.relation.code != program.syntax[index].relation.code || link.body != program.links[index].body ||
			link.variable != program.links[index].variable || !link.validFor(syntax.body, syntax.variable) {
			t.Fatalf("dependency freeze did not retain immutable tier-2 handle %d", index+1)
		}
	}
	if program.relationDependencyFreeze.observability != program.formalRegion ||
		!program.relationDependencyFreeze.evaluator.validFor(program) {
		t.Fatal("full-result-v1 did not bind its observable quotient and evaluator")
	}
	want := cloneFormalRelationEquations(program.formalTemplate.equations)

	// A run may only borrow frozen equations. Repeated equation lookup must not
	// mutate the template retained by RelationProgram.
	for iteration := 0; iteration < 32; iteration++ {
		for _, cell := range program.formalRegion.cells {
			equation, ok := program.formalTemplate.equation(cell)
			if !ok || !equation.Cell.valid() {
				t.Fatalf("iteration %d lost equation for %+v", iteration, cell)
			}
		}
	}
	if !reflect.DeepEqual(program.formalTemplate.equations, want) {
		for index := range want {
			if !reflect.DeepEqual(program.formalTemplate.equations[index], want[index]) {
				t.Fatalf("repeated formal-template reads mutated equation %d: got %#v want %#v", index, program.formalTemplate.equations[index], want[index])
			}
		}
		t.Fatal("repeated formal-template reads changed equation inventory")
	}

	second, err := FreezeRelationProgram([]RelationProgramUnit{unit}, topology)
	if err != nil {
		t.Fatal(err)
	}
	assertFormalTemplateOwnedByProgram(t, second)
	if second.formalTemplate == program.formalTemplate || second.formalRegion == program.formalRegion {
		t.Fatal("independent relation programs shared formal template ownership")
	}
	if !reflect.DeepEqual(program.formalTemplate.equations, want) {
		t.Fatal("a repeated program freeze mutated the first formal template")
	}
}

func TestFreezeRelationProgramBracketsFullResultV1Telemetry(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("relation-program-dependency-freeze-telemetry"))
	body := lexicalidentity.RootBody(namespace)
	unit := formalTemplateFreezeUnit(t, body)
	telemetry := &FreezeTelemetry{}
	program, err := FreezeRelationProgramWithTelemetry([]RelationProgramUnit{unit}, testAcyclicCallTopology(t, body), telemetry)
	if err != nil {
		t.Fatal(err)
	}
	if !program.relationDependencyFreeze.validFor(program) || telemetry.DependencyFreeze.Calls != 1 ||
		telemetry.DependencyFreeze.Elapsed < 0 {
		t.Fatalf("dependency-freeze telemetry = %#v, product valid = %t", telemetry.DependencyFreeze, program.relationDependencyFreeze.validFor(program))
	}
}

func TestFreezeRelationProgramBuildsDetachedNonReusableTier1Manifest(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("relation-program-tier-1-manifest"))
	body := lexicalidentity.RootBody(namespace)
	unit := formalTemplateFreezeUnit(t, body)
	topology := testAcyclicCallTopology(t, body)

	program, err := FreezeRelationProgram([]RelationProgramUnit{unit}, topology)
	if err != nil {
		t.Fatal(err)
	}
	manifest := program.tier1
	if !manifest.Valid() || manifest.Reusable() {
		t.Fatalf("tier-1 manifest validity/reuse = %v/%v, want valid and fail-closed", manifest.Valid(), manifest.Reusable())
	}
	if len(manifest.units) != 1 || manifest.units[0].body != body || manifest.units[0].graphID != unit.Graph.ID() {
		t.Fatalf("tier-1 manifest body directory = %#v, want detached record for %s", manifest.units, body)
	}
	if len(manifest.missing) == 0 {
		t.Fatal("tier-1 manifest silently treated opaque execution authorities as versioned")
	}
	exported, ok := program.Tier1Manifest()
	if !ok || !reflect.DeepEqual(exported, manifest) {
		t.Fatal("tier-1 manifest export is not an exact detached copy")
	}
	exported.units[0].domainLanes = append(exported.units[0].domainLanes, "foreign")
	again, ok := program.Tier1Manifest()
	if !ok || reflect.DeepEqual(exported, again) {
		t.Fatal("tier-1 manifest export retained program-owned slice storage")
	}
	before := manifest
	unit.NodeReads = append(unit.NodeReads, []cfg.Point{unit.Graph.Entry()})
	unit.Definitions = append(unit.Definitions, RelationProgramDefinition{Target: body, Point: unit.Graph.Entry()})
	if !reflect.DeepEqual(program.tier1, before) {
		t.Fatal("tier-1 manifest retained mutable unit workspace")
	}
}

func formalTemplateFreezeUnit(t *testing.T, body lexicalidentity.StableLexicalBodyID) RelationProgramUnit {
	t.Helper()
	reg := standard.Registry()
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	code := wir.NewBody("formal-template")
	code.AssignDebugPointOrdinals(graph)
	plan := operationplan.New(graph, factflow.FactsInput{}).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	surface, err := operationplan.SealCallSurface(body, graph.Size(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan = plan.WithCallSurface(surface)
	resolver := visibility.NewResolver(nil)
	paths := factapply.NewPathSemanticAuthority(resolver, nil, nil)
	return RelationProgramUnit{
		Body: body, Registry: reg, KeySpace: resolver.KeySpace(), Graph: graph, Plan: plan,
		Shape: Shape{}, Domain: state.RegisteredProductDomain(reg), PathSemantics: paths,
		Returns:       factapply.NewReturnAuthority(paths, plan.Facts()),
		EntrySeedPlan: state.NewEntrySeedPlan(nil), InitialStatePlan: testInitialStatePlan(t, body, graph),
	}
}

func assertFormalTemplateOwnedByProgram(t *testing.T, program *RelationProgram) {
	t.Helper()
	if program == nil || program.formalRegion == nil || program.formalTemplate == nil ||
		program.formalTemplate.region != program.formalRegion ||
		len(program.formalTemplate.equations) != len(program.formalRegion.cells) {
		t.Fatal("frozen relation program has no exact formal template ownership")
	}
	for _, equation := range program.formalTemplate.equations {
		if !equation.Cell.valid() || equation.Cell.region != program.formalRegion {
			t.Fatalf("foreign formal equation owner: %#v", equation.Cell)
		}
		for _, input := range equation.Inputs {
			if !input.valid(equation.Cell) || input.Source.region != program.formalRegion {
				t.Fatalf("foreign formal input owner: %#v", input)
			}
		}
	}
}

func cloneFormalRelationEquations(in []formalRelationEquation) []formalRelationEquation {
	out := append([]formalRelationEquation(nil), in...)
	for index := range out {
		out[index].Inputs = make([]formalRelationTemplateInput, len(in[index].Inputs))
		copy(out[index].Inputs, in[index].Inputs)
	}
	return out
}
