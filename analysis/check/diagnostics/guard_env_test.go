package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
)

func TestGuardEnvWithoutDescendantFactsPreservesRootOnlyFacts(t *testing.T) {
	root := path.NewPath(1, "x")
	descendant := path.NewPath(2, "box").Field("value")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: descendant, value: "ready"},
			{target: root, value: "string"},
		},
		typeChecks: []runtimeTypeConstraint{
			{target: descendant, name: "string"},
			{target: root, name: "string"},
		},
		present: []path.Path{descendant, root},
	}

	got := env.withoutDescendantFacts()
	if len(got.constraints) != 1 || !got.constraints[0].target.Equal(root) || got.constraints[0].value != "string" {
		t.Fatalf("constraints = %#v, want only root literal constraint", got.constraints)
	}
	if len(got.typeChecks) != 1 || !got.typeChecks[0].target.Equal(root) || got.typeChecks[0].name != "string" {
		t.Fatalf("type checks = %#v, want only root runtime type check", got.typeChecks)
	}
	if len(got.present) != 1 || !got.present[0].Equal(root) {
		t.Fatalf("present = %#v, want only root presence", got.present)
	}
}
