package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TestInferredUncalledReturnJoinsAlternatives states the derived contract of a
// body whose branches return different concrete values: each return occurrence
// is one alternative of the same slot, so the contract is their join.
func TestInferredUncalledReturnJoinsAlternatives(t *testing.T) {
	stringTarget, ok := shapefact.EncodeTarget(typ.String)
	if !ok {
		t.Fatal("encode string return witness")
	}
	closure := equation.OutputClosure{Outcomes: append(
		returnCandidate("op-00000004", string(stringTarget)),
		returnCandidate("op-00000009", `scalar/string/"empty"`)...,
	)}
	result, derived := inferredUncalledReturnType(closure)
	if !derived {
		t.Fatal("a body with two string return sites derived no contract")
	}
	if !typ.TypeEquals(result, typ.String) {
		t.Fatalf("joined return contract = %v, want string", result)
	}
}

// TestInferredUncalledReturnRefusesUnwitnessedAlternative keeps the join
// fail-closed: one return site with no concrete witness states no contract for
// the whole callable.
func TestInferredUncalledReturnRefusesUnwitnessedAlternative(t *testing.T) {
	closure := equation.OutputClosure{Outcomes: append(
		returnCandidate("op-00000004", `scalar/string/"ready"`),
		returnCandidate("op-00000009", "scalar/top")...,
	)}
	if result, derived := inferredUncalledReturnType(closure); derived {
		t.Fatalf("an unwitnessed return alternative derived contract %v", result)
	}
}

// TestInferredUncalledReturnRefusesMultipleSlots holds the arity boundary: a
// tuple result is not a single-slot callable contract.
func TestInferredUncalledReturnRefusesMultipleSlots(t *testing.T) {
	closure := equation.OutputClosure{Outcomes: returnCandidate("op-00000004", `scalar/string/"ready"`, "scalar/bool/true")}
	if result, derived := inferredUncalledReturnType(closure); derived {
		t.Fatalf("a two-slot return derived contract %v", result)
	}
}
