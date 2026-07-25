package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func providerCallEquation(name, provider string, arguments ...string) equation.Equation {
	operands := []equation.Operand{
		{Role: "application", Term: equation.ClosedTerm([]byte("call/" + name))},
		{Role: "provider", Term: equation.ClosedTerm([]byte(provider))},
		{Role: "argument-display-00000000", Term: equation.ClosedTerm([]byte("display"))},
	}
	for index, argument := range arguments {
		operands = append(operands, equation.Operand{
			Role: callArgumentRole(index),
			Term: equation.ClosedTerm([]byte(argument)),
		})
	}
	return equation.Equation{
		Target:     equation.Coordinate{Name: name},
		Occurrence: equation.Occurrence{Kind: "call-results"},
		Operands:   operands,
	}
}

func callArgumentRole(index int) string {
	digits := []byte("00000000")
	for position := len(digits) - 1; position >= 0 && index > 0; position-- {
		digits[position] = byte('0' + index%10)
		index /= 10
	}
	return "argument-" + string(digits)
}

func TestContextualCallbackArgumentSelectsTheConsumingPosition(t *testing.T) {
	artifact := equation.Artifact{Equations: []equation.Equation{
		providerCallEquation("op-1", "provider/module/v1/json", "scalar/string/\"{}\"", "temp/24", "temp/26"),
	}}

	provider, position, arguments, located := contextualCallbackArgument(artifact, "temp/26")
	if !located {
		t.Fatalf("the call consuming temp/26 was not located")
	}
	if string(provider) != "provider/module/v1/json" {
		t.Fatalf("provider %q, want the call's own provider", provider)
	}
	if position != 2 {
		t.Fatalf("position %d, want 2", position)
	}
	if len(arguments) != 3 || string(arguments[1]) != "temp/24" {
		t.Fatalf("arguments %v, want every term the call supplies", arguments)
	}
}

func TestContextualCallbackArgumentRefusesATermTwoCallsConsume(t *testing.T) {
	artifact := equation.Artifact{Equations: []equation.Equation{
		providerCallEquation("op-1", "provider/module/v1/json", "temp/26"),
		providerCallEquation("op-2", "provider/module/v1/process", "temp/26"),
	}}

	if _, _, _, located := contextualCallbackArgument(artifact, "temp/26"); located {
		t.Fatalf("a term reaching two calls states no single contextual contract")
	}
}

func TestContextualCallbackArgumentRefusesACallWithNoProvider(t *testing.T) {
	call := providerCallEquation("op-1", "provider/module/v1/json", "temp/26")
	operands := make([]equation.Operand, 0, len(call.Operands))
	for _, operand := range call.Operands {
		if operand.Role != "provider" {
			operands = append(operands, operand)
		}
	}
	call.Operands = operands

	if _, _, _, located := contextualCallbackArgument(equation.Artifact{Equations: []equation.Equation{call}}, "temp/26"); located {
		t.Fatalf("a call with no resolved provider states no declared callback contract")
	}
}

func TestOpenTypeParameterRefusesAnUnboundParameterInsideARecursiveGraph(t *testing.T) {
	parameter := typ.NewTypeParam("U", nil)
	node := typetable.NewRecord().Field("id", typ.String).Field("payload", parameter).Build()
	node.Fields = append(node.Fields, typ.Field{Name: "children", Type: typ.NewArray(node)})

	if !openTypeParameter(node, map[typ.Type]bool{}) {
		t.Fatalf("an unbound parameter inside a recursive record was not reported")
	}
}

func TestOpenTypeParameterAcceptsAFullyBoundRecursiveGraph(t *testing.T) {
	node := typetable.NewRecord().Field("id", typ.String).Build()
	node.Fields = append(node.Fields, typ.Field{Name: "children", Type: typ.NewArray(node)})

	if openTypeParameter(node, map[typ.Type]bool{}) {
		t.Fatalf("a recursive graph with no parameter left was reported open")
	}
}

func TestContextualCallableContractPairsInstantiatedParametersWithTheDerivedResult(t *testing.T) {
	outcome := equation.OutputClosure{Outcomes: []equation.Fact{
		{Key: "return-candidate/op-1/arity", Value: []byte("1")},
		{Key: "return-candidate/op-1/0", Value: []byte("scalar/string/\"label\"")},
	}}
	record := typetable.NewRecord().Field("id", typ.String).Build()

	contract, stated := contextualCallableContract([]typ.Type{record}, outcome)
	if !stated {
		t.Fatalf("a body with one derived result states no contract")
	}
	function, callable := contract.(*typ.Function)
	if !callable || len(function.Params) != 1 || len(function.Returns) != 1 {
		t.Fatalf("contract %v, want one parameter and one result", contract)
	}
	if !typ.TypeEquals(function.Params[0].Type, record) {
		t.Fatalf("parameter %v, want the instantiated call-site type", function.Params[0].Type)
	}
	if function.Returns[0] == nil || function.Returns[0].Kind() != typ.String.Kind() {
		t.Fatalf("result %v, want the derived string result", function.Returns[0])
	}
}

func TestContextualCallableContractRefusesABodyWithNoDerivedResult(t *testing.T) {
	record := typetable.NewRecord().Field("id", typ.String).Build()

	if _, stated := contextualCallableContract([]typ.Type{record}, equation.OutputClosure{}); stated {
		t.Fatalf("a body deriving no result states no callable contract")
	}
}

func TestQualifiedChildDiagnosticKeyKeepsTheProvingBody(t *testing.T) {
	nested := "child/aabb/type.assignment/op-00000004"
	if got := qualifiedChildDiagnosticKey("ccdd", nested); got != nested {
		t.Fatalf("relayed key %q, want the proving body's own qualification", got)
	}
	if got := qualifiedChildDiagnosticKey("ccdd", "type.assignment/op-1"); got != "child/ccdd/type.assignment/op-1" {
		t.Fatalf("qualified key %q, want this body named as the prover", got)
	}
}
