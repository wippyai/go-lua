package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func returnContractResult(t *testing.T, declared typ.Type, returned typ.Type) equation.TransactionResult {
	t.Helper()
	declaredValue, ok := shapefact.EncodeTarget(declared)
	if !ok {
		t.Fatal("encode declared return")
	}
	returnedValue, ok := shapefact.EncodeTarget(returned)
	if !ok {
		t.Fatal("encode returned value")
	}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: "value/path/result/op-00000001", Value: returnedValue},
		{Key: epochFactPrefix + "path/result/op-00000001", Value: []byte("op-00000001")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	operation := equation.BoundEquation{
		Target: equation.Coordinate{Body: equation.BodyID{1}, Name: "op-00000002"},
		Operands: []equation.BoundOperand{
			{Role: "declared-return-0", Value: declaredValue},
			{Role: "return-value-00000000", Value: []byte("path/result")},
		},
	}
	result, err := publicationKernel(operation, partition)
	if err != nil {
		t.Fatalf("publication kernel: %v", err)
	}
	return result
}

// TestGenericReturnSchemaRefutesNothing states that a declared return whose
// type parameter survives inside an application is a schema over every
// instantiation, not a concrete contract: a body evaluated at one
// instantiation has nothing to refute against.
func TestGenericReturnSchemaRefutesNothing(t *testing.T) {
	parameter := typ.NewTypeParam("T", nil)
	event := typetable.NewRecord().Field("id", typ.String).Build()
	result := returnContractResult(t,
		typ.Instantiate(ambient.ChannelGeneric(), parameter),
		typ.Instantiate(ambient.ChannelGeneric(), event))
	if len(result.Closure.Diagnostics) != 0 {
		t.Fatalf("Channel<T> refuted an instantiated return: %#v", result.Closure.Diagnostics)
	}
}

// TestConcreteReturnContractStillRefutes keeps that exemption falsifiable: a
// return type with no open parameter decides the slot as before.
func TestConcreteReturnContractStillRefutes(t *testing.T) {
	result := returnContractResult(t, typ.String, typ.Number)
	if len(result.Closure.Diagnostics) == 0 {
		t.Fatal("a declared string return admitted a number")
	}
}

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
