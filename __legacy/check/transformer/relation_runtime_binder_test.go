package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestBindRealRelationBodyBindsEveryFrozenOccurrence(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	body := lexicalidentity.RootBody(namespace)
	program, err := FreezeRelationProgram([]RelationProgramUnit{formalTemplateFreezeUnit(t, body)}, testAcyclicCallTopology(t, body))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := program.BindRealRelationBody(body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	occurrences := binding.Occurrences()
	if len(occurrences) == 0 {
		t.Fatal("real body has no bound occurrences")
	}
	artifact, err := binding.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(artifact.Equations) != len(occurrences) {
		t.Fatalf("compiled equations = %d, want %d bound occurrences", len(artifact.Equations), len(occurrences))
	}
	for _, occurrence := range occurrences {
		if occurrence.Ordinal == 0 || occurrence.Target.Body != equation.BodyID(body) {
			t.Fatalf("malformed bound occurrence: %#v", occurrence)
		}
		if len(occurrence.Operands) == 0 {
			t.Fatalf("occurrence %d omitted runtime operands", occurrence.Ordinal)
		}
		for _, operand := range occurrence.Operands {
			if len(operand.Value) == 0 || !strings.Contains(string(operand.Value), "relation-runtime-slot/v1") {
				t.Fatalf("occurrence %d has unbound %s operand: %#v", occurrence.Ordinal, operand.Role, operand)
			}
		}
	}
}

func TestRealRelationBodyBinderNamesExactUnboundOccurrence(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	body := lexicalidentity.RootBody(namespace)
	program, err := FreezeRelationProgram([]RelationProgramUnit{formalTemplateFreezeUnit(t, body)}, testAcyclicCallTopology(t, body))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := program.BindRealRelationBody(body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = binding.Binder()(RelationEquationOccurrence{Body: body, Kind: OperatorEntry, Ordinal: 999})
	if err == nil || !strings.Contains(err.Error(), "/999") {
		t.Fatalf("unbound occurrence error = %v, want exact ordinal", err)
	}
}

func TestRealRelationBodyBinderCompletesExternalCallHistoricalFibersLocally(t *testing.T) {
	program, _, _, _ := formalCallOutcomeFiberFixture(t)
	body := program.bodies[0].body
	// This is deliberately a binder-only source row. The production template
	// and its dependency inventory have already frozen; completing this view
	// must not create a new production PublishedRead edge.
	program.bodies[0].relation.code.publication.points = []relationPointPublication{{point: 7, ref: 1}}
	for equationIndex := range program.formalTemplate.equations {
		for stageIndex := range program.formalTemplate.equations[equationIndex].StepStages {
			plan := program.formalTemplate.equations[equationIndex].StepStages[stageIndex].Operator.externalCall
			if plan != nil {
				plan.access = []valueAccessTerm{{term: 1, point: 7, hasPoint: true}}
			}
		}
	}

	binding, err := program.BindRealRelationBody(body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	if gaps := binding.BindingGaps(); len(gaps) != 0 {
		t.Fatalf("binder-local historical point had gaps: %#v", gaps)
	}
	for _, occurrence := range binding.Occurrences() {
		if occurrence.Kind != OperatorExternalCall {
			continue
		}
		for _, fiber := range occurrence.ExternalCallFibers {
			if fiber.Point == cfg.Point(7) {
				if !fiber.Historical || len(fiber.Ordinals) == 0 {
					t.Fatalf("historical fiber = %#v", fiber)
				}
				return
			}
		}
		t.Fatalf("external-call occurrence omitted binder-local historical point: %#v", occurrence)
	}
	t.Fatal("fixture omitted ExternalCall occurrence")
}

func TestRealRelationBodyBinderNamesUnpublishedHistoricalExternalCallFiber(t *testing.T) {
	program, _, _, _ := formalCallOutcomeFiberFixture(t)
	body := program.bodies[0].body
	for equationIndex := range program.formalTemplate.equations {
		for stageIndex := range program.formalTemplate.equations[equationIndex].StepStages {
			plan := program.formalTemplate.equations[equationIndex].StepStages[stageIndex].Operator.externalCall
			if plan != nil {
				plan.access = []valueAccessTerm{{term: 1, point: 7, hasPoint: true}}
			}
		}
	}
	binding, err := program.BindRealRelationBody(body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	gaps := binding.BindingGaps()
	if len(gaps) != 1 || gaps[0].Family != "external-call-historical-fiber" || gaps[0].Point != cfg.Point(7) {
		t.Fatalf("historical fiber gaps = %#v", gaps)
	}
}
