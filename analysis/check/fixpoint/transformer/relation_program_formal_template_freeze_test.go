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
