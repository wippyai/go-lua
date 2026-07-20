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
	transactionLanes, err := externalCallTransactionLanes(domain.Lanes(), capability)
	if err != nil {
		return state.TransferAccess{}, err
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
	for _, role := range capability.FieldRoles() {
		writesOperands, known := externalCallFieldWritesOperandValues(role.FieldName)
		if !known {
			return state.TransferAccess{}, fmt.Errorf("factapply: unclassified CallOutcome Value write field %q", role.FieldName)
		}
		if writesOperands {
			valueWrites = append(valueWrites, inputs[primary].Values...)
			break
		}
	}
	return state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs: inputs, ValueWrites: valueWrites,
		LaneCarryReads: transactionLanes, LaneWrites: transactionLanes,
		ValueCarry: primary, LaneCarry: primary, DiagnosticCarry: primary, ReachableCarry: -1,
		ReachableWrites: true,
	})
}

func externalCallFieldWritesOperandValues(field string) (bool, bool) {
	switch field {
	case "NormalReturnFacts", "ParamPathRefinements", "ParamPathWrites",
		"ParamPathInvalidations", "ParamConditions", "ParamPathRelations", "ParamExposures":
		return true, true
	case "Results", "PostReturnAuthority", "SuspensionKnown", "MaySuspend",
		"ProtectedCallTypestate", "HeapTableObjects", "Placements",
		"ParamObligations", "PathObligations", "TypestateRequirements",
		"ParamLengthFloors", "ReturnConditionRefinements", "ReturnConditionSlots",
		"ReturnPresenceRelations":
		return false, true
	default:
		return false, false
	}
}

func externalCallTransactionLanes(enabled state.LaneSet, capability callpayload.CallOutcomeCapability) (state.LaneSet, error) {
	selected := state.NewLaneSet(state.LaneUserLattices)
	for _, role := range capability.FieldRoles() {
		lanes, known := externalCallFieldLanes(role.FieldName)
		if !known {
			return state.LaneSet{}, fmt.Errorf("factapply: unclassified CallOutcome field %q", role.FieldName)
		}
		selected = selected.With(lanes...)
	}
	for _, lane := range selected.IDs() {
		if !enabled.Has(lane) {
			return state.LaneSet{}, fmt.Errorf("factapply: external-call field requires unregistered lane %q", lane)
		}
	}
	return selected, nil
}

func externalCallFieldLanes(field string) ([]state.LaneID, bool) {
	switch field {
	case "Results", "PostReturnAuthority", "SuspensionKnown", "MaySuspend",
		"ParamObligations", "PathObligations", "TypestateRequirements",
		"ReturnConditionRefinements", "ReturnConditionSlots", "ReturnPresenceRelations":
		return nil, true
	case "ProtectedCallTypestate":
		return []state.LaneID{state.LaneTypestates}, true
	case "HeapTableObjects":
		return []state.LaneID{state.LaneHeapTableIdentity}, true
	case "Placements":
		return []state.LaneID{state.LanePlacement}, true
	case "ParamLengthFloors":
		return []state.LaneID{state.LaneLenFloors}, true
	case "ParamConditions", "ParamPathRelations":
		return []state.LaneID{state.LanePathEvidence, state.LaneStoreRelations, state.LaneDiffRelations}, true
	case "ParamExposures":
		return []state.LaneID{state.LanePathEvidence, state.LaneDynamicIndex, state.LaneHeapTableIdentity, state.LanePlacement}, true
	case "ParamPathRefinements", "ParamPathWrites", "ParamPathInvalidations":
		return []state.LaneID{
			state.LanePathEvidence, state.LaneDynamicIndex, state.LaneHeapTableIdentity,
			state.LaneFrozenTables, state.LaneStoreRelations, state.LaneKeyMemberships,
			state.LaneLenFloors, state.LaneNumFloors, state.LaneNumCeils, state.LaneDiffRelations,
		}, true
	case "NormalReturnFacts":
		return []state.LaneID{
			state.LanePathEvidence, state.LaneDynamicIndex, state.LaneHeapTableIdentity,
			state.LaneFrozenTables, state.LaneEffectDeltas, state.LaneEscapeEvents,
			state.LaneChannelSelect, state.LaneStoreRelations, state.LaneKeyMemberships,
			state.LaneTypestates, state.LanePlacement, state.LaneLenFloors,
			state.LaneNumFloors, state.LaneNumCeils, state.LaneDiffRelations,
		}, true
	default:
		return nil, false
	}
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
