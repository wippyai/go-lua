package programartifact

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestFunctionBoundaryScalarShapesAndPackRefinementFailClosed(t *testing.T) {
	bodyID, bodyContext, entryID := valuesLawID(1), valuesLawID(2), valuesLawID(3)
	functionID, callFormal := valuesLawID(4), valuesLawID(5)
	formal := FunctionFormalPort{id: valuesLawID(6), cell: valuesLawID(7), storage: valuesLawID(8)}
	vararg := FunctionVarargPort{id: valuesLawID(9), cell: valuesLawID(10)}
	capture := FunctionCapturePort{
		id: valuesLawID(11), inner: valuesLawID(12), outer: valuesLawID(13),
		innerBody: bodyID, outerBody: valuesLawID(14),
	}
	outcome := valuesLawID(15)
	boundary := FunctionBoundaryRow{
		id: functionID, body: bodyID, bodyContext: bodyContext, entry: entryID, callFormal: callFormal,
		formals: []FunctionFormalPort{formal}, vararg: vararg, hasVararg: true,
		captures: []FunctionCapturePort{capture}, outcomes: []keyspace.ContentID{outcome}, sealed: true,
	}
	if !boundary.Available() {
		t.Fatal("valid function boundary unavailable")
	}
	mutations := []FunctionBoundaryRow{boundary, boundary, boundary, boundary, boundary}
	mutations[0].formals = append([]FunctionFormalPort(nil), boundary.formals...)
	mutations[0].formals[0].position = 1
	mutations[1].captures = append([]FunctionCapturePort(nil), boundary.captures...)
	mutations[1].captures[0].outerBody = bodyID
	mutations[2].hasVararg = false
	mutations[3].outcomes = nil
	mutations[4].entry = keyspace.ContentID{}
	for index, mutated := range mutations {
		if mutated.Available() {
			t.Fatalf("malformed function boundary mutation %d was available", index)
		}
	}

	body := BodyRow{
		id: bodyID, context: bodyContext, entry: entryID, function: functionID, formal: callFormal,
		callable: true, outcomeEnd: 1, sealed: true,
	}
	packFormal := PackFormalCell{formal: formal.id, cell: formal.cell, storage: formal.storage}
	receipt := PackReceipt{bodies: []PackBodyReceiptRow{{id: bodyID, context: bodyContext, formals: []PackFormalCell{packFormal}, callable: true}}}
	artifact := &Artifact{bodies: []BodyRow{body}, functionBoundaries: []FunctionBoundaryRow{boundary}}
	if !receipt.validAgainst(artifact) {
		t.Fatal("exact Pack formal refinement rejected")
	}
	spliced := receipt
	spliced.bodies = append([]PackBodyReceiptRow(nil), receipt.bodies...)
	spliced.bodies[0].formals = append([]PackFormalCell(nil), receipt.bodies[0].formals...)
	spliced.bodies[0].formals[0].storage = valuesLawID(16)
	if spliced.validAgainst(artifact) {
		t.Fatal("Pack formal with a substituted storage port refined the generic boundary")
	}
}
