package assemble

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestStaticCatalogKeepsOnlyDeliverableTargetEndpoints(t *testing.T) {
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"escape_assemble"}}},
		ValuesVars: 1,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesVariable, Var: 0},
		Outcomes: []target.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: target.ValuesSpec{Tail: target.ValuesClosed}},
		},
		Transfers: []target.TransferSpec{{
			Endpoint: target.TransferEndpoint{Kind: target.TransferEndpointInput, Input: 0},
			Payload:  target.InputSource{Kind: target.InputSourceValueFormal},
			Alias:    target.InputSource{Kind: target.InputSourceValueFormal},
			Identity: target.TransferIdentitySame, Capabilities: target.TransferCapabilitiesPreserveAll,
			Outcomes: []target.TransferOutcomeSpec{
				{Outcome: 0, Possibility: target.TransferMayDeliver},
				{Outcome: 1, Possibility: target.TransferMayReject},
			},
		}, {
			Endpoint: target.TransferEndpoint{Kind: target.TransferEndpointExternal},
			Payload:  target.InputSource{Kind: target.InputSourceValueFormal},
			Alias:    target.InputSource{Kind: target.InputSourceValueFormal},
			Identity: target.TransferIdentityUnspecified, Capabilities: target.TransferCapabilitiesUnspecified,
			Outcomes: []target.TransferOutcomeSpec{
				{Outcome: 0, Possibility: target.TransferMayReject},
				{Outcome: 1, Possibility: target.TransferMayDeliver},
			},
		}},
		Effects: target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	program, err := lower.Lower(lower.Source{Name: "escape_assemble.lua", Text: []byte("return nil")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "escape_assemble", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	catalog, ok := buildStaticCatalog(linked)
	if !ok {
		t.Fatal("build Escape static catalog")
	}
	expected := 0
	expectedByOperation := make(map[target.Operation]int)
	for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
		operation, operationOK := contract.OperationAt(operationIndex)
		if !operationOK {
			t.Fatalf("operation %d", operationIndex)
		}
		for transferIndex := 0; transferIndex < contract.TransferCount(operation); transferIndex++ {
			for outcomeIndex := 0; outcomeIndex < contract.TransferOutcomeCount(operation, transferIndex); outcomeIndex++ {
				_, possibility, outcomeOK := contract.TransferOutcomeAt(operation, transferIndex, outcomeIndex)
				if !outcomeOK {
					t.Fatalf("outcome %d/%d", transferIndex, outcomeIndex)
				}
				if possibility&target.TransferMayDeliver != 0 {
					expected++
					expectedByOperation[operation]++
				}
			}
		}
	}
	if got := len(catalog.transfers); got != expected {
		t.Fatalf("deliverable endpoints = %d, want %d", got, expected)
	}
	for operation, want := range expectedByOperation {
		if got := len(catalog.byOperation[operation]); got != want {
			t.Fatalf("operation endpoints = %d, want %d", got, want)
		}
	}
	for _, row := range catalog.transfers {
		if owner, ownerOK := contract.TransferOwner(row.transfer); !ownerOK || owner != row.operation {
			t.Fatal("catalog transfer owner")
		}
		if !row.target.Available() || !row.endpoint.Available() || !row.operand.ContentID().Available() {
			t.Fatal("catalog lost target-owned endpoint identity")
		}
	}
}
