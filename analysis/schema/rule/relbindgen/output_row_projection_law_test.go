package relbindgen

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestOutputDestinationProjectsSourceCoordinateIntoDeclaredRelation(t *testing.T) {
	content := func(label string) identity.ContentID {
		t.Helper()
		value, ok := identity.DeriveContentID("relbindgen/output-row-projection-law/v1", []byte(label))
		if !ok {
			t.Fatalf("derive %q", label)
		}
		return value
	}
	owner, ok := model.IssueOwnerID(content("owner"))
	if !ok {
		t.Fatal("owner")
	}
	inputRelation, inputOK := model.IssueRelationID(owner, content("relation/input"))
	outputRelation, outputOK := model.IssueRelationID(owner, content("relation/output"))
	inputKey, inputKeyOK := model.IssueKeyID(inputRelation, content("key/input"))
	outputKey, outputKeyOK := model.IssueKeyID(outputRelation, content("key/output"))
	inputDenominator, inputDenominatorOK := model.NewDenominatorRef(inputRelation, inputKey)
	outputDenominator, outputDenominatorOK := model.NewDenominatorRef(outputRelation, outputKey)
	inputColumn, inputColumnOK := model.IssueColumnID(inputRelation, content("column/input"))
	outputColumn, outputColumnOK := model.IssueColumnID(outputRelation, content("column/output"))
	typeID, typeOK := model.IssueTypeID(owner, content("type"))
	schemaID, schemaOK := model.IssueSchemaID(owner, content("schema"))
	operationID, operationOK := model.IssueOperationID(owner, content("operation"))
	if !inputOK || !outputOK || !inputKeyOK || !outputKeyOK || !inputDenominatorOK || !outputDenominatorOK ||
		!inputColumnOK || !outputColumnOK || !typeOK || !schemaOK || !operationOK {
		t.Fatal("schema identities")
	}

	coordinate := content("coordinate")
	inputRow, inputRowOK := model.IssueRowID(inputRelation, coordinate)
	outputRow, outputRowOK := model.IssueRowID(outputRelation, coordinate)
	if !inputRowOK || !outputRowOK || inputRow == outputRow {
		t.Fatal("nominal rows")
	}
	fence, ok := binding.NewFence(schemaID, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	scope, ok := issuer.IssueScope(content("scope"))
	if !ok {
		t.Fatal("scope")
	}
	inputMembership, inputMembershipOK := binding.NewMembershipView(inputRelation, []model.RowID{inputRow})
	outputMembership, outputMembershipOK := binding.NewMembershipView(outputRelation, []model.RowID{outputRow})
	inputWitness, inputWitnessOK := issuer.IssueDenominator(inputDenominator, inputMembership, content("witness/input"))
	outputWitness, outputWitnessOK := issuer.IssueDenominator(outputDenominator, outputMembership, content("witness/output"))
	if !inputMembershipOK || !outputMembershipOK || !inputWitnessOK || !outputWitnessOK {
		t.Fatal("denominator witnesses")
	}
	inputAddress, ok := issuer.IssueCell(inputWitness, scope, inputColumn, inputRow)
	if !ok {
		t.Fatal("input address")
	}
	value, ok := issuer.IssueValue(typeID, content("value"))
	if !ok {
		t.Fatal("value")
	}
	present, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	inputCell, ok := binding.NewCell(inputAddress, typeID, value, present)
	if !ok {
		t.Fatal("input cell")
	}
	destination, ok := binding.NewScalarDestination(inputCell)
	if !ok {
		t.Fatal("source destination")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	operation, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Outputs: []signature.Output{{
			Relation: outputRelation, Column: outputColumn, Type: typeID,
			Presence: signature.ProducePresent, Denominator: outputDenominator,
		}},
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("operation")
	}
	bufferValue, ok := binding.NewProposalBuffer(operation, fence, []binding.DenominatorWitness{outputWitness}, scope, destination)
	if !ok {
		t.Fatal("proposal buffer")
	}
	outputs := Outputs{
		declared: operation.Outputs(), buffer: &bufferValue, issuer: issuer,
		row: inputRow, presence: present,
	}
	declared, got, ok := outputs.destination(0, typeID)
	if !ok {
		t.Fatal("project destination")
	}
	if declared.Relation != outputRelation || got.Relation() != outputRelation || got.Row() != outputRow {
		t.Fatalf("destination relation/row = %v/%v, want %v/%v", got.Relation(), got.Row(), outputRelation, outputRow)
	}
	if got.Row() == inputRow || got.Witness().Same(inputWitness) {
		t.Fatal("output retained the input relation authority")
	}
}
