package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SealExternalCallTransferAccess binds the canonical external-call
// transaction to the provider's already-compiled ValueTerm footprint. Values
// remain finite; residual lanes are classified exhaustively here beside the
// State transaction, never inferred by the transformer.
func SealExternalCallTransferAccess(
	domain state.ProductDomain,
	inputs []state.TransferInputAccess,
	inputPoints []cfg.Point,
	primary int,
	capability callpayload.CallOutcomeCapability,
	resultWrites []statekey.Value,
) (state.TransferAccess, error) {
	if len(inputs) == 0 || len(inputPoints) != len(inputs) || primary < 0 || primary >= len(inputs) {
		return state.TransferAccess{}, fmt.Errorf("factapply: external-call transfer has invalid input roles")
	}
	transactionLanes := capability.TransactionLanes()
	for _, lane := range domain.CallBoundaryFactorLanes() {
		transactionLanes = transactionLanes.With(lane.ID())
	}
	for _, lane := range transactionLanes.IDs() {
		if !domain.Lanes().Has(lane) {
			return state.TransferAccess{}, fmt.Errorf("factapply: external-call field requires unregistered lane %q", lane)
		}
	}
	inputs = cloneTransferInputs(inputs)
	inputs[primary].Lanes = inputs[primary].Lanes.With(capability.PrimaryInputLanes().IDs()...)
	inputs[primary].TypestateResourceQueries = append(
		inputs[primary].TypestateResourceQueries,
		capability.TypestateResourceQueries()...,
	)
	for index, point := range inputPoints {
		if index != primary {
			lanes, err := capability.ReadInputLanes(point)
			if err != nil {
				return state.TransferAccess{}, err
			}
			inputs[index].Lanes = inputs[index].Lanes.With(lanes.IDs()...)
		}
	}
	valueWrites := append([]statekey.Value(nil), resultWrites...)
	if capability.OperandValueWrites() {
		valueWrites = append(valueWrites, inputs[primary].Values...)
	}
	return state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs: inputs, ValueWrites: valueWrites,
		LaneCarryReads: transactionLanes, LaneWrites: transactionLanes,
		ValueCarry: primary, LaneCarry: primary, DiagnosticCarry: primary, ReachableCarry: -1,
		ReachableWrites: true,
	})
}

// SealGenericForTransferAccess binds the exact generic-for target transaction
// and its registered iterator lane law to the canonical term footprint.
func SealGenericForTransferAccess(
	domain state.ProductDomain,
	inputs []state.TransferInputAccess,
	pointEntry, current int,
	op GenericForOperation,
) (state.TransferAccess, error) {
	if len(inputs) == 0 || pointEntry < 0 || pointEntry >= len(inputs) || current < 0 || current >= len(inputs) {
		return state.TransferAccess{}, fmt.Errorf("factapply: generic-for transfer has invalid input roles")
	}
	transaction, valid := PlanGenericForTransaction(op)
	if !valid {
		return state.TransferAccess{}, fmt.Errorf("factapply: generic-for transfer has no transaction")
	}
	iterator, hasIterator := op.Iterator()
	indexedValue := hasIterator && iterator.Kind == iteration.IterateIndexed && op.VariableIndex() == 1
	sourceReads, currentReads, writes, err := domain.GenericForTransferLanes(indexedValue)
	if err != nil {
		return state.TransferAccess{}, err
	}
	inputs = cloneTransferInputs(inputs)
	inputs[pointEntry].Lanes = inputs[pointEntry].Lanes.With(sourceReads.IDs()...)
	inputs[current].Lanes = inputs[current].Lanes.With(currentReads.IDs()...)
	return state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs: inputs, ValueWrites: []statekey.Value{statekey.SymbolValue(transaction.Target)},
		LaneCarryReads: writes, LaneWrites: writes,
		ValueCarry: current, LaneCarry: current, DiagnosticCarry: current, ReachableCarry: -1,
		ReachableWrites: true,
	})
}

func cloneTransferInputs(inputs []state.TransferInputAccess) []state.TransferInputAccess {
	out := make([]state.TransferInputAccess, len(inputs))
	for index, input := range inputs {
		out[index] = state.TransferInputAccess{
			Values:                   append([]statekey.Value(nil), input.Values...),
			Lanes:                    state.NewLaneSet(input.Lanes.IDs()...),
			TypestateResourceQueries: append([]state.TypestateResourceQuery(nil), input.TypestateResourceQueries...),
			Diagnostics:              input.Diagnostics,
			Reachable:                input.Reachable,
		}
	}
	return out
}
