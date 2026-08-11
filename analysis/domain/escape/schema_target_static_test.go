package escape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestTransferBoundaryIsTargetStatic(t *testing.T) {
	source, contract, operation, transfer := staticTransferSource(t)
	heapSchema, heapOK := heap.Seal(source)
	schema, schemaOK := NewSchema(source, heapSchema)
	if !heapOK || !schemaOK {
		t.Fatal("seal target-static Escape schema")
	}
	coordinate, coordinateOK := schema.CoordinateForTransfer(transfer)
	if !coordinateOK || !coordinate.Valid() {
		t.Fatal("static transfer coordinate")
	}
	kind, kindOK := schema.BoundaryKind(coordinate)
	if !kindOK || kind != BoundaryTransfer {
		t.Fatalf("transfer coordinate kind = %d/%v", kind, kindOK)
	}
	if owner, ownerOK := contract.TransferOwner(transfer); !ownerOK || owner != operation {
		t.Fatal("fixture transfer owner")
	}
	if _, ok := schema.CoordinateForTransfer(target.TransferID(0)); ok {
		t.Fatal("fabricated transfer entered target-static Escape schema")
	}
}

func TestTransferBoundaryDoesNotMultiplyPerCallApplication(t *testing.T) {
	baseSource, baseContract, _, baseTransfer := staticTransferSource(t)
	baseHeap, baseHeapOK := heap.Seal(baseSource)
	baseSchema, baseSchemaOK := NewSchema(baseSource, baseHeap)
	if !baseHeapOK || !baseSchemaOK {
		t.Fatal("seal no-call target-static Escape schema")
	}
	if _, coordinateOK := baseSchema.CoordinateForTransfer(baseTransfer); !coordinateOK {
		t.Fatal("no-call static transfer coordinate")
	}
	source, contract, operation, transfer := staticTransferSourceText(t, `
local function invoke() end
invoke()
invoke()
return nil
`)
	callCount := 0
	for index := 0; index < source.Project().Applications().Count(); index++ {
		application, applicationOK := source.Project().Applications().At(index)
		if !applicationOK {
			t.Fatalf("application %d", index)
		}
		if _, _, call := source.Project().Applications().Call(application); !call {
			continue
		}
		callCount++
		if !source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
			t.Fatalf("call application %d did not admit fixture operation", index)
		}
	}
	if callCount < 2 {
		t.Fatalf("call application count = %d, want at least 2", callCount)
	}
	heapSchema, heapOK := heap.Seal(source)
	schema, schemaOK := NewSchema(source, heapSchema)
	if !heapOK || !schemaOK {
		t.Fatal("seal multi-application target-static Escape schema")
	}
	coordinate, coordinateOK := schema.CoordinateForTransfer(transfer)
	if !coordinateOK || !coordinate.Valid() {
		t.Fatal("static transfer coordinate after multiple calls")
	}
	transferBoundaries := transferBoundaryCount(t, schema)
	baseTransferBoundaries := transferBoundaryCount(t, baseSchema)
	if transferBoundaries != totalTransfers(t, contract) || baseTransferBoundaries != totalTransfers(t, baseContract) || transferBoundaries != baseTransferBoundaries {
		t.Fatalf("transfer boundary counts calls=%d base=%d, want one per sealed TransferID (%d), not %d call applications", transferBoundaries, baseTransferBoundaries, totalTransfers(t, contract), callCount)
	}
}

func transferBoundaryCount(t testing.TB, schema Schema) int {
	t.Helper()
	count := 0
	for index := 0; index < schema.CoordinateCount(); index++ {
		candidate, candidateOK := schema.CoordinateAt(index)
		kind, kindOK := schema.BoundaryKind(candidate)
		if !candidateOK || !kindOK {
			t.Fatalf("schema boundary %d", index)
		}
		if kind == BoundaryTransfer {
			count++
		}
	}
	return count
}

func totalTransfers(t testing.TB, contract *target.Contract) int {
	t.Helper()
	if contract == nil {
		t.Fatal("nil contract")
	}
	count := 0
	for index := 0; index < contract.OperationCount(); index++ {
		operation, operationOK := contract.OperationAt(index)
		if !operationOK {
			t.Fatalf("operation %d", index)
		}
		count += contract.TransferCount(operation)
	}
	return count
}

func staticTransferSource(t testing.TB) (*link.Link, *target.Contract, target.Operation, target.TransferID) {
	return staticTransferSourceText(t, "return nil")
}

func staticTransferSourceText(t testing.TB, text string) (*link.Link, *target.Contract, target.Operation, target.TransferID) {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:   []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"escape_static_transfer"}}},
		ValuesVars: 1,
		Input:      target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesVariable, Var: 0},
		Outcomes:   []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Transfers: []target.TransferSpec{{
			Endpoint:     target.TransferEndpoint{Kind: target.TransferEndpointExternal},
			Payload:      target.InputSource{Kind: target.InputSourceValueFormal},
			Alias:        target.InputSource{Kind: target.InputSourceValueFormal},
			Identity:     target.TransferIdentityUnspecified,
			Capabilities: target.TransferCapabilitiesUnspecified,
			Outcomes:     []target.TransferOutcomeSpec{{Outcome: 0, Possibility: target.TransferMayDeliver}},
		}},
		Effects: target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := lower.Lower(lower.Source{Name: "escape_static_transfer.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "escape_static_transfer", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	operation, operationOK := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"escape_static_transfer"}})
	if !operationOK {
		t.Fatal("transfer operation")
	}
	transfer, transferOK := contract.TransferIDAt(operation, 0)
	if !transferOK {
		t.Fatal("transfer identity")
	}
	return source, contract, operation, transfer
}
