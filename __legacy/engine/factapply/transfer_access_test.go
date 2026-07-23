package factapply

import (
	"testing"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestExternalCallTransferAccessIsFiniteAndExcludesUnrelatedSlots(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	read := statekey.SymbolValue(symbol.ID(41))
	result := statekey.ReturnSlot(0)
	unrelated := statekey.SymbolValue(symbol.ID(99))
	program := callpayload.SealCallOutcomeProgram(
		"result-only transfer test", []string{"Results"},
		state.NewLaneSet(), state.NewLaneSet(), nil, nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			return callpayload.CallOutcome{}, nil
		},
	)
	prepared, err := program.PrepareSite(transfer.NodeContext{Registry: domain.Registry(), Point: 1}, factflow.CallSiteView{})
	if err != nil {
		t.Fatal(err)
	}
	capability := prepared.Capability()
	access, err := SealExternalCallTransferAccess(domain, []state.TransferInputAccess{{
		Values: []statekey.Value{read}, Lanes: state.NewLaneSet(state.LanePathEvidence),
	}, {}, {}}, []cfg.Point{1, 2, 3}, 0, capability, []statekey.Value{result})
	if err != nil {
		t.Fatal(err)
	}
	primary, ok := access.ProviderInput(0)
	if !ok || !hasTransferSlot(primary.Values, read) || hasTransferSlot(primary.Values, unrelated) {
		t.Fatalf("primary = %#v/%t", primary, ok)
	}
	for index := 1; index < access.ProviderInputCount(); index++ {
		input, ok := access.ProviderInput(index)
		if !ok || len(input.Values) != 0 || input.Lanes.Len() != 0 {
			t.Fatalf("unrelated input %d = %#v/%t", index, input, ok)
		}
	}
	if writes := access.ValueWrites(); hasTransferSlot(writes, read) || !hasTransferSlot(writes, result) || hasTransferSlot(writes, unrelated) {
		t.Fatalf("external writes = %v", writes)
	}
}

func TestExternalCallTransferAccessSeparatesProviderReadsFromTransactionLanes(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	program := callpayload.SealCallOutcomeProgram(
		"transfer access test", []string{"ProtectedCallTypestate"},
		state.NewLaneSet(), state.NewLaneSet(), nil, nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			return callpayload.CallOutcome{}, nil
		},
	)
	ctx := transfer.NodeContext{Registry: domain.Registry(), Point: 1}
	prepared, err := program.PrepareSite(ctx, factflow.CallSiteView{})
	if err != nil {
		t.Fatal(err)
	}
	capability := prepared.Capability()
	access, err := SealExternalCallTransferAccess(
		domain, []state.TransferInputAccess{{}}, []cfg.Point{1}, 0, capability, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := access.ProviderInput(0)
	if input.Lanes.Has(state.LaneUserLattices) || input.Lanes.Has(state.LaneTypestates) || input.Lanes.Len() != 0 {
		t.Fatalf("output-only lanes leaked into provider input = %v", input.Lanes.IDs())
	}
	inputProgram, err := callpayload.PrepareExternalCallInputProgram(
		domain, access, []cfg.Point{1}, 0,
		func(root statekey.Value) (statekey.Value, bool) { return root, root != 0 },
	)
	if err != nil {
		t.Fatal(err)
	}
	layout, ok := inputProgram.Layout(0)
	if !ok || len(layout.Lanes()) != 0 {
		t.Fatalf("output-only lanes leaked into provider layout = %v/%v", layout.Lanes(), ok)
	}
	for _, lanes := range []state.LaneSet{access.LaneCarryReads(), access.LaneWrites()} {
		if !lanes.Has(state.LaneUserLattices) || !lanes.Has(state.LaneTypestates) || lanes.Has(state.LaneChannelSelect) || lanes.Has(state.LaneEffectDeltas) {
			t.Fatalf("external transaction lanes = %v", lanes.IDs())
		}
	}
	factorProgram, err := PrepareExternalCallFactorProgram(
		domain, access, cfg.Point(1), nil,
		func(point, result uint32) (statekey.Value, bool) { return statekey.CallResult(point, result), true },
	)
	if err != nil {
		t.Fatal(err)
	}
	committed := state.NewLaneSet()
	for _, lane := range factorProgram.Lanes() {
		committed = committed.With(lane.ID())
	}
	if !committed.Has(state.LaneUserLattices) || !committed.Has(state.LaneTypestates) {
		t.Fatalf("output-only lanes absent from commit program = %v", committed.IDs())
	}
}

func TestGenericForTransferAccessSelectsOnlyDeclaredTransactionLanes(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	target := symbol.ID(73)
	op, ok := NewGenericForOperation(0, target, target, nil, nil)
	if !ok {
		t.Fatal("generic-for operation did not seal")
	}
	read := statekey.SymbolValue(symbol.ID(72))
	access, err := SealGenericForTransferAccess(domain, []state.TransferInputAccess{{
		Values: []statekey.Value{read}, Lanes: state.NewLaneSet(state.LaneHeapTableIdentity),
	}, {}}, 0, 1, op)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTransferSlot(access.ValueWrites(), statekey.SymbolValue(target)) {
		t.Fatalf("generic-for writes = %v", access.ValueWrites())
	}
	writes := access.LaneWrites()
	if !writes.Has(state.LaneKeyMemberships) || writes.Has(state.LaneChannelSelect) || writes.Has(state.LaneEffectDeltas) {
		t.Fatalf("generic-for lane writes = %v", writes.IDs())
	}
	pointEntry, _ := access.ProviderInput(0)
	if !hasTransferSlot(pointEntry.Values, read) || !pointEntry.Lanes.Has(state.LaneHeapTableIdentity) || pointEntry.Lanes.Has(state.LaneKeyMemberships) {
		t.Fatalf("generic-for point entry = %#v", pointEntry)
	}
	current, _ := access.ProviderInput(1)
	if hasTransferSlot(current.Values, read) || current.Lanes.Has(state.LaneHeapTableIdentity) || current.Lanes.Has(state.LaneChannelSelect) || !current.Lanes.Has(state.LaneKeyMemberships) {
		t.Fatalf("generic-for current = %#v", current)
	}
}

func hasTransferSlot(values []statekey.Value, want statekey.Value) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
