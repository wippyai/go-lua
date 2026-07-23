package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
