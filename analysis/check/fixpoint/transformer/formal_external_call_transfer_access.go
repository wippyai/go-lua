package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// bindExternalCallAccessToDeclaredInputs preserves every compiler-sealed read
// while binding a source without a PublishedRead wire to the primary input
// frame. The primary tuple is already available to the provider; this is a
// declaration completion, not a new production dependency.
func bindExternalCallAccessToDeclaredInputs(access []valueAccessTerm, inputPoints []cfg.Point) []valueAccessTerm {
	if len(inputPoints) == 0 {
		return nil
	}
	declared := make(map[cfg.Point]struct{}, len(inputPoints))
	for _, point := range inputPoints {
		declared[point] = struct{}{}
	}
	primary := inputPoints[0]
	out := cloneValueAccessTerms(access)
	for index := range out {
		if !out[index].hasPoint {
			continue
		}
		if _, present := declared[out[index].point]; !present {
			out[index].point = primary
		}
	}
	return out
}

// externalCallTransferAccess seals the provider's exact observations directly
// from the canonical ValueTerm DAG. It is the sole bridge from relation syntax
// to the carrier-neutral external-call factor programs; no executor-specific
// access vocabulary is retained.
func externalCallTransferAccess(
	body *relationProgramBody,
	step boundaryPrefixStep,
	inputPoints []cfg.Point,
	inputCount, primary int,
	capability callpayload.CallOutcomeCapability,
) (state.TransferAccess, error) {
	return externalCallTransferAccessWithAccess(body, step, step.access, inputPoints, inputCount, primary, capability)
}

// externalCallTransferAccessWithAccess seals one leaf-owned subset of the
// compiler-frozen source access row.  The public helper above remains the
// whole-site authority used by output/commit preparation.
func externalCallTransferAccessWithAccess(
	body *relationProgramBody,
	step boundaryPrefixStep,
	access []valueAccessTerm,
	inputPoints []cfg.Point,
	inputCount, primary int,
	capability callpayload.CallOutcomeCapability,
) (state.TransferAccess, error) {
	if body == nil || step.kind != boundaryPrefixExternalCall ||
		inputCount <= 0 || primary < 0 || primary >= inputCount || len(inputPoints) != inputCount {
		return state.TransferAccess{}, fmt.Errorf("transformer: external-call transfer access is unowned")
	}
	inputs, err := externalCallTransferInputs(body, access, inputPoints, inputCount, primary)
	if err != nil {
		return state.TransferAccess{}, err
	}
	writes, err := body.valueTermReadSlots(step.writes...)
	if err != nil {
		return state.TransferAccess{}, err
	}
	if step.memberCall.site != 0 {
		memberReads, readErr := body.valueTermReadSlots(step.memberCall.receiver, step.memberCall.provider)
		if readErr != nil {
			return state.TransferAccess{}, readErr
		}
		inputs[primary].Values = append(inputs[primary].Values, memberReads...)
		inputs[primary].Diagnostics = true
	}
	return factapply.SealExternalCallTransferAccess(
		body.productDomain, inputs, inputPoints, primary, capability, writes,
	)
}

// externalCallTransferAccessWithoutDynamicOperandLanes seals the provider
// frame authority after removing only operand-owned dynamic-read lanes.  The
// capability is still passed to factapply, so a provider that explicitly owns
// a dynamic lane retains it.  This lets the formal adapter bind a complete
// provider frame from static roots while the operand evaluator borrows the
// dynamic lanes through its demand cursor.
func externalCallTransferAccessWithoutDynamicOperandLanes(
	body *relationProgramBody,
	step boundaryPrefixStep,
	access []valueAccessTerm,
	inputPoints []cfg.Point,
	inputCount, primary int,
	capability callpayload.CallOutcomeCapability,
) (state.TransferAccess, error) {
	if body == nil || step.kind != boundaryPrefixExternalCall ||
		inputCount <= 0 || primary < 0 || primary >= inputCount || len(inputPoints) != inputCount {
		return state.TransferAccess{}, fmt.Errorf("transformer: external-call provider access is unowned")
	}
	inputs, err := externalCallTransferInputs(body, access, inputPoints, inputCount, primary)
	if err != nil {
		return state.TransferAccess{}, err
	}
	dynamic, err := body.productDomain.DynamicReadPotentialLanes()
	if err != nil {
		return state.TransferAccess{}, err
	}
	for index := range inputs {
		kept := state.LaneSet{}
		for _, lane := range inputs[index].Lanes.IDs() {
			if !dynamic.Has(lane) {
				kept = kept.With(lane)
			}
		}
		inputs[index].Lanes = kept
	}
	writes, err := body.valueTermReadSlots(step.writes...)
	if err != nil {
		return state.TransferAccess{}, err
	}
	if step.memberCall.site != 0 {
		memberReads, readErr := body.valueTermReadSlots(step.memberCall.receiver, step.memberCall.provider)
		if readErr != nil {
			return state.TransferAccess{}, readErr
		}
		inputs[primary].Values = append(inputs[primary].Values, memberReads...)
		inputs[primary].Diagnostics = true
	}
	return factapply.SealExternalCallTransferAccess(
		body.productDomain, inputs, inputPoints, primary, capability, writes,
	)
}

func externalCallTransferInputs(
	body *relationProgramBody,
	access []valueAccessTerm,
	inputPoints []cfg.Point,
	inputCount, primary int,
) ([]state.TransferInputAccess, error) {
	if body == nil || inputCount <= 0 || primary < 0 || primary >= inputCount || len(inputPoints) != inputCount {
		return nil, fmt.Errorf("transformer: external-call provider access has invalid input roles")
	}
	inputs := make([]state.TransferInputAccess, inputCount)
	for _, item := range access {
		direct, err := externalCallTransferInput(body, item)
		if err != nil {
			return nil, err
		}
		if len(direct.Values) == 0 && direct.Lanes.Len() == 0 && !direct.Diagnostics && !direct.Reachable {
			continue
		}
		matched := false
		if !item.hasPoint {
			mergeExternalCallTransferInput(&inputs[0], direct)
			matched = true
		} else {
			for index, point := range inputPoints {
				if point == item.point {
					mergeExternalCallTransferInput(&inputs[index], direct)
					matched = true
				}
			}
		}
		if !matched {
			return nil, fmt.Errorf("transformer: external-call provider term at point %d has no declared read input", item.point)
		}
	}
	return inputs, nil
}

func externalCallTransferInput(body *relationProgramBody, access valueAccessTerm) (state.TransferInputAccess, error) {
	if body == nil || body.relation.arena == nil || access.term == 0 || int(access.term) >= len(body.relation.arena.values) {
		return state.TransferInputAccess{}, fmt.Errorf("transformer: external-call provider access has a foreign term")
	}
	switch body.relation.arena.values[access.term].op {
	case valueLuaTypeName, valueDynamicRead, valueDynamicTableRead:
		return body.valueTermFactorAccess(access.term)
	default:
		return body.valueTermNodeFactorAccess(access.term)
	}
}

func mergeExternalCallTransferInput(destination *state.TransferInputAccess, source state.TransferInputAccess) {
	destination.Values = append(destination.Values, source.Values...)
	destination.Lanes = destination.Lanes.With(source.Lanes.IDs()...)
	destination.Diagnostics = destination.Diagnostics || source.Diagnostics
	destination.Reachable = destination.Reachable || source.Reachable
}
