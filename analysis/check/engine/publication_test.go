package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestPublishedValuesChoosesDependencyLatestContentNotKeyOrder(t *testing.T) {
	var body equation.BodyID
	body[0] = 1
	first := equation.Coordinate{Body: body, Name: "z-first"}
	second := equation.Coordinate{Body: body, Name: "a-second"}
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	write := func(target equation.Coordinate, dependencies []equation.Coordinate) equation.Equation {
		return equation.Equation{
			Target: target, Entry: entry, Dependencies: dependencies,
			Occurrence: equation.Occurrence{Kind: "environment-write", ContractID: equation.ContentID{1}}, KernelID: "test/write",
			Operands: []equation.Operand{
				{Role: equation.MustOperandRole("target"), Term: equation.ClosedTerm([]byte("path/value"))},
				{Role: equation.MustOperandRole("display"), Term: equation.ClosedTerm([]byte("value"))},
			},
		}
	}
	artifact := equation.Artifact{Equations: []equation.Equation{write(first, nil), write(second, []equation.Coordinate{first})}}
	stored := []equation.Fact{
		{Key: "value/path/value/z-first", Value: []byte("scalar/number/1")},
		{Key: "value/path/value/a-second", Value: []byte("scalar/number/2")},
	}
	values := publishedValues(artifact, stored)
	if len(values) != 1 || values[0].Key != "value" || string(values[0].Value) != "2" {
		t.Fatalf("published values = %#v, want the dependency-latest value 2", values)
	}

	artifact.Equations[0], artifact.Equations[1] = artifact.Equations[1], artifact.Equations[0]
	values = publishedValues(artifact, stored)
	if len(values) != 1 || string(values[0].Value) != "2" {
		t.Fatalf("reordered artifact published values = %#v, want dependency-latest value 2", values)
	}
}
