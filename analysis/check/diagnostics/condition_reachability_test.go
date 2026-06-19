package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func TestLiteralConditionProofSkipsIrrelevantNegatedConstraints(t *testing.T) {
	target := path.NewPath(1, "status")
	check := branchcond.Check{
		Kind:    branchcond.CheckLiteralEqual,
		Path:    target,
		Literal: guardLiteral("ready"),
	}
	proof, ok := literalConditionProof(guardEnv{constraints: []literalConstraint{
		{target: target, value: guardLiteral("stale"), negated: true},
		{target: target, value: guardLiteral("ready")},
	}}, check, false)
	if !ok {
		t.Fatalf("literalConditionProof returned no proof, want proof from later exact literal constraint")
	}
	if !proof.always {
		t.Fatalf("proof.always = false, want true")
	}
	if !strings.Contains(proof.proven, "status is \"ready\"") {
		t.Fatalf("proof.proven = %q, want exact matching literal evidence", proof.proven)
	}
}

func TestLiteralConditionProofTruthTableForNegatedChecks(t *testing.T) {
	target := path.NewPath(1, "status")
	cases := []struct {
		name              string
		constraintNegated bool
		checkNegated      bool
		wantAlways        bool
	}{
		{name: "known_equal_then_equals", wantAlways: true},
		{name: "known_equal_then_not_equals", checkNegated: true},
		{name: "known_not_equal_then_equals", constraintNegated: true},
		{name: "known_not_equal_then_not_equals", constraintNegated: true, checkNegated: true, wantAlways: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			kind := branchcond.CheckLiteralEqual
			if tt.checkNegated {
				kind = branchcond.CheckLiteralNot
			}
			check := branchcond.Check{
				Kind:    kind,
				Path:    target,
				Literal: guardLiteral("ready"),
			}
			proof, ok := literalConditionProof(guardEnv{constraints: []literalConstraint{{
				target:  target,
				value:   guardLiteral("ready"),
				negated: tt.constraintNegated,
			}}}, check, tt.checkNegated)
			if !ok {
				t.Fatalf("literalConditionProof returned no proof")
			}
			if proof.always != tt.wantAlways {
				t.Fatalf("proof.always = %v, want %v; proof=%#v", proof.always, tt.wantAlways, proof)
			}
		})
	}
}

func TestLiteralConditionProofFailsClosedOnConflictingConstraints(t *testing.T) {
	target := path.NewPath(1, "status")
	check := branchcond.Check{
		Kind:    branchcond.CheckLiteralEqual,
		Path:    target,
		Literal: guardLiteral("ready"),
	}
	if proof, ok := literalConditionProof(guardEnv{constraints: []literalConstraint{
		{target: target, value: guardLiteral("stale")},
		{target: target, value: guardLiteral("ready")},
	}}, check, false); ok {
		t.Fatalf("literalConditionProof = %#v, want no proof for contradictory same-path literal facts", proof)
	}
}
