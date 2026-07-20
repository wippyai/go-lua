package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

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
	if body == nil || step.kind != boundaryPrefixExternalCall ||
		inputCount <= 0 || primary < 0 || primary >= inputCount || len(inputPoints) != inputCount {
		return state.TransferAccess{}, fmt.Errorf("transformer: external-call transfer access is unowned")
	}
	inputs, err := externalCallTransferInputs(body, step.access, inputPoints, inputCount, primary)
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
	return body.valueTermNodeFactorAccess(access.term)
}

func mergeExternalCallTransferInput(destination *state.TransferInputAccess, source state.TransferInputAccess) {
	destination.Values = append(destination.Values, source.Values...)
	destination.Lanes = destination.Lanes.With(source.Lanes.IDs()...)
	destination.Diagnostics = destination.Diagnostics || source.Diagnostics
	destination.Reachable = destination.Reachable || source.Reachable
}
