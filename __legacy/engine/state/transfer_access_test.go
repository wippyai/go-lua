package state

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestTransferAccessSealsExactDetachedInventory(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	first, second := statekey.SymbolValue(symbol.ID(11)), statekey.SymbolValue(symbol.ID(12))
	access, err := SealTransferAccess(domain, TransferAccessConfig{
		ProviderInputs: []TransferInputAccess{{Values: []statekey.Value{second, first, second}, Lanes: NewLaneSet(LanePathEvidence), Reachable: true}},
		ValueWrites:    []statekey.Value{second}, LaneCarryReads: NewLaneSet(LanePathEvidence), LaneWrites: NewLaneSet(LanePathEvidence),
		ValueCarry: 0, LaneCarry: 0, DiagnosticCarry: 0, ReachableCarry: -1, ReachableWrites: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	input, ok := access.ProviderInput(0)
	if !ok || len(input.Values) != 2 || input.Values[0] != first || input.Values[1] != second || !input.Lanes.Has(LanePathEvidence) {
		t.Fatalf("sealed input = %#v/%v", input, ok)
	}
	input.Values[0] = second
	again, _ := access.ProviderInput(0)
	if again.Values[0] != first {
		t.Fatal("caller mutation changed sealed transfer access")
	}
	if access.LaneWrites().Has(LaneChannelSelect) {
		t.Fatal("unrelated lane entered sealed transfer access")
	}
}

func TestTransferAccessRejectsMalformedAndUnregisteredInventory(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	base := TransferAccessConfig{
		ProviderInputs: []TransferInputAccess{{}},
		ValueCarry:     0, LaneCarry: 0, DiagnosticCarry: 0, ReachableCarry: 0,
	}
	zero := base
	zero.ProviderInputs[0].Values = []statekey.Value{0}
	if _, err := SealTransferAccess(domain, zero); err == nil {
		t.Fatal("zero Value slot was silently omitted")
	}
	unknown := base
	unknown.ProviderInputs = []TransferInputAccess{{Lanes: NewLaneSet(LaneID("unregistered"))}}
	if _, err := SealTransferAccess(domain, unknown); err == nil {
		t.Fatal("unregistered read lane was accepted")
	}
	unknownWrite := base
	unknownWrite.LaneCarryReads = NewLaneSet(LaneID("unregistered"))
	unknownWrite.LaneWrites = NewLaneSet(LaneID("unregistered"))
	if _, err := SealTransferAccess(domain, unknownWrite); err == nil {
		t.Fatal("unregistered write lane was accepted")
	}
}
