package factapply

import (
	"testing"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestExternalCallInputProgramMatchesDirectProviderReads(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	primaryPoint, historicalPoint := cfg.Point(1801), cfg.Point(1799)
	primarySlot, historicalSlot := statekey.SymbolValue(101), statekey.SymbolValue(102)
	access, err := state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs: []state.TransferInputAccess{
			{Values: []statekey.Value{primarySlot}, Lanes: state.NewLaneSet(state.LaneHeapTableIdentity), Diagnostics: true, Reachable: true},
			{Values: []statekey.Value{historicalSlot}, Lanes: state.NewLaneSet(state.LaneTypestates)},
		},
		ValueCarry: 0, LaneCarry: 0, DiagnosticCarry: 0, ReachableCarry: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := callpayload.PrepareExternalCallInputProgram(
		domain, access, []cfg.Point{primaryPoint, historicalPoint}, 0,
		func(slot statekey.Value) (statekey.Value, bool) { return slot, slot != 0 },
	)
	if err != nil {
		t.Fatal(err)
	}
	primaryValue := typevalue.LiteralString(reg, "primary")
	historicalValue := typevalue.LiteralString(reg, "historical")
	inputs := []state.State{
		state.Reachable(state.State{}).WriteValue(reg, primarySlot, primaryValue),
		state.Reachable(state.State{}).WriteValue(reg, historicalSlot, historicalValue),
	}
	primaryDiagnostics := callpayload.DiagnosticOutput{SuspensionKnown: true, MaySuspend: true}
	frame, err := callpayload.BindConcreteExternalCallInputFrame(
		&program, inputs, []callpayload.DiagnosticOutput{primaryDiagnostics, {}},
	)
	if err != nil {
		t.Fatal(err)
	}

	primary, primaryLayout, ok := frame.Primary()
	if !ok {
		t.Fatal("primary external-call input wire is absent")
	}
	ordinal, ok := primaryLayout.ValueOrdinal(primarySlot)
	if !ok {
		t.Fatal("primary Values root is absent from sealed layout")
	}
	gotPrimary, ok := primary.Value(ordinal)
	if !ok || !product.Equal(reg, gotPrimary, inputs[0].ReadValue(reg, primarySlot)) {
		t.Fatal("factor-native primary provider read differs from direct State read")
	}
	if !primary.Diagnostics().Equal(reg, primaryDiagnostics) || !primary.Reachable() {
		t.Fatal("declared primary diagnostics/reachability were not projected")
	}
	if len(primary.Factors()) != 1 || primary.Factors()[0].Lane().ID() != state.LaneHeapTableIdentity {
		t.Fatalf("primary factor inventory = %#v", primary.Factors())
	}

	history, layouts := frame.Historical(historicalPoint)
	if len(history) != 1 || len(layouts) != 1 {
		t.Fatalf("historical wires = %d/%d", len(history), len(layouts))
	}
	ordinal, ok = layouts[0].ValueOrdinal(historicalSlot)
	if !ok {
		t.Fatal("historical Values root is absent from sealed layout")
	}
	gotHistorical, ok := history[0].Value(ordinal)
	if !ok || !product.Equal(reg, gotHistorical, inputs[1].ReadValue(reg, historicalSlot)) {
		t.Fatal("factor-native historical provider read differs from direct State read")
	}
	if !history[0].Diagnostics().Empty() || history[0].Reachable() {
		t.Fatal("undeclared historical diagnostic/reachability fibers leaked into provider frame")
	}
	if len(history[0].Factors()) != 1 || history[0].Factors()[0].Lane().ID() != state.LaneTypestates {
		t.Fatalf("historical factor inventory = %#v", history[0].Factors())
	}
	providerInput, err := frame.BindCallOutcomeInput(callpayload.CallOutcomeValueOperands{
		Receiver: primaryValue, HasReceiver: true,
		Arguments: []callpayload.CallOutcomeArgumentOperand{{Value: historicalValue, Present: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	gotReceiver, hasReceiver := providerInput.Receiver()
	gotArgument, hasArgument := providerInput.Argument(0)
	if !hasReceiver || !hasArgument || !product.Equal(reg, gotReceiver, primaryValue) ||
		!product.Equal(reg, gotArgument, historicalValue) {
		t.Fatal("evaluated ValueTerm roles did not survive provider-input binding")
	}
	if len(providerInput.Historical(historicalPoint)) != 1 || len(providerInput.Primary().Factors()) != 1 {
		t.Fatal("provider input lost its sealed primary/historical factor wires")
	}
}

func TestExternalCallInputProgramRejectsAuthorityWidening(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	slot := statekey.SymbolValue(111)
	access, err := state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs: []state.TransferInputAccess{{Values: []statekey.Value{slot}}},
		ValueCarry:     0, LaneCarry: 0, DiagnosticCarry: 0, ReachableCarry: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	program, err := callpayload.PrepareExternalCallInputProgram(
		domain, access, []cfg.Point{1901}, 0,
		func(slot statekey.Value) (statekey.Value, bool) { return slot, true },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := program.BindFrame([]callpayload.ExternalCallInputWireOperands{{
		Values: []product.Value{product.Bottom(reg)}, Diagnostics: callpayload.DiagnosticOutput{MaySuspend: true},
	}}); err == nil {
		t.Fatal("external-call input frame accepted undeclared diagnostics")
	}
}
