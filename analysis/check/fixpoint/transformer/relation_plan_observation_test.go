package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestRelationBodyPlanObservationsExposeFrozenWTOClassification(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	acyclic := lexicalidentity.RootBody(namespace)
	cyclic := lexicalidentity.FunctionBody(namespace, 1)
	topology, err := operationplan.SealCallTopology(
		[]lexicalidentity.StableLexicalBodyID{acyclic, cyclic}, nil,
		[]operationplan.CallTopologyComponentInput{{Body: cyclic, Component: 1}},
		[]operationplan.CallTopologyBoundaryInput{{Body: acyclic}, {Body: cyclic}},
	)
	if err != nil {
		t.Fatal(err)
	}
	program, err := FreezeRelationProgram([]RelationProgramUnit{
		formalTemplateFreezeUnit(t, acyclic),
		formalTemplateFreezeUnit(t, cyclic),
	}, topology)
	if err != nil {
		t.Fatal(err)
	}
	observations := program.BodyPlanObservations()
	if len(observations) != 2 {
		t.Fatalf("body observations = %d, want 2", len(observations))
	}
	for _, want := range []struct {
		body    lexicalidentity.StableLexicalBodyID
		acyclic bool
	}{{acyclic, true}, {cyclic, false}} {
		got, ok := program.BodyAcyclic(want.body)
		if !ok || got != want.acyclic {
			t.Fatalf("BodyAcyclic(%s) = %t/%t, want %t/true", want.body, got, ok, want.acyclic)
		}
		found := false
		for _, observation := range observations {
			if observation.Body == want.body {
				found = true
				if observation.Acyclic != want.acyclic {
					t.Fatalf("observation(%s).Acyclic = %t, want %t", want.body, observation.Acyclic, want.acyclic)
				}
			}
		}
		if !found {
			t.Fatalf("body %s absent from observations", want.body)
		}
	}
	if _, ok := program.BodyAcyclic(lexicalidentity.FunctionBody(namespace, 99)); ok {
		t.Fatal("foreign body classified")
	}
}
