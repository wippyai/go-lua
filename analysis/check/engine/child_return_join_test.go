package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func returnCandidate(name string, values ...string) []equation.Fact {
	facts := []equation.Fact{{Key: "return-candidate/" + name + "/arity", Value: []byte(fmt.Sprint(len(values)))}}
	for index, value := range values {
		facts = append(facts, equation.Fact{Key: fmt.Sprintf("return-candidate/%s/%08d", name, index), Value: []byte(value)})
	}
	return facts
}

// TestChildReturnValuesJoinsIdenticalAlternatives proves that two reachable
// return statements publishing the same tuple are one result, not a choice the
// caller must reject.
func TestChildReturnValuesJoinsIdenticalAlternatives(t *testing.T) {
	closure := equation.OutputClosure{Outcomes: append(
		returnCandidate("op-00000004", "scalar/number/1"),
		returnCandidate("op-00000009", "scalar/number/1")...,
	)}
	returns, err := childReturnValues(closure, true)
	if err != nil {
		t.Fatalf("identical return alternatives were not joined: %v", err)
	}
	if len(returns) != 1 || string(returns[0]) != "scalar/number/1" {
		t.Fatalf("joined return = %#v, want one scalar/number/1", returns)
	}
}

// TestChildReturnValuesRejectsDisagreeingAlternatives keeps the join
// falsifiable: differing slot values remain an unresolved alternative.
func TestChildReturnValuesRejectsDisagreeingAlternatives(t *testing.T) {
	closure := equation.OutputClosure{Outcomes: append(
		returnCandidate("op-00000004", "scalar/number/1"),
		returnCandidate("op-00000009", "scalar/number/2")...,
	)}
	if _, err := childReturnValues(closure, true); !errors.Is(err, errMultipleChildReturnAlternatives) {
		t.Fatalf("disagreeing return alternatives were joined: %v", err)
	}
}

// TestChildReturnValuesRejectsDisagreeingArity is the tuple-shape counterpart:
// a different arity is never one result.
func TestChildReturnValuesRejectsDisagreeingArity(t *testing.T) {
	closure := equation.OutputClosure{Outcomes: append(
		returnCandidate("op-00000004", "scalar/number/1"),
		returnCandidate("op-00000009", "scalar/number/1", "scalar/bool/true")...,
	)}
	if _, err := childReturnValues(closure, true); !errors.Is(err, errMultipleChildReturnAlternatives) {
		t.Fatalf("return alternatives of different arity were joined: %v", err)
	}
}

// TestChildReturnValuesKeepsSelectAlternativesUnjoined preserves the select
// rule: its arms are evaluated in separate partitions, so their branch-local
// tuples are not comparable at the call boundary even when they agree.
func TestChildReturnValuesKeepsSelectAlternativesUnjoined(t *testing.T) {
	closure := equation.OutputClosure{Outcomes: append(
		returnCandidate("op-00000004", "scalar/number/1"),
		returnCandidate("op-00000009", "scalar/number/1")...,
	)}
	if _, err := childReturnValues(closure, false); !errors.Is(err, errMultipleChildReturnAlternatives) {
		t.Fatalf("select return alternatives were joined: %v", err)
	}
}
