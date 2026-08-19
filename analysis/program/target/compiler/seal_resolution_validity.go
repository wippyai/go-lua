package compiler

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func validAuthoredInputSource(source vocabulary.InputSource, valueFormals int, valuesVars uint32) bool {
	switch source.Kind {
	case vocabulary.InputSourceValueFormal:
		return uint64(source.Ordinal) < uint64(valueFormals)
	case vocabulary.InputSourceValuesVar:
		return uint64(source.Ordinal) < uint64(valuesVars)
	default:
		return false
	}
}

func validTransferPossibility(possibility vocabulary.TransferPossibility) bool {
	const valid = vocabulary.TransferMayDeliver | vocabulary.TransferMayReject
	return possibility != 0 && possibility&^valid == 0
}

func validTransferEndpoint(endpoint vocabulary.TransferEndpoint, valueFormals int) bool {
	switch endpoint.Kind {
	case vocabulary.TransferEndpointInput:
		return uint64(endpoint.Input) < uint64(valueFormals)
	case vocabulary.TransferEndpointExternal:
		return endpoint.Input == 0
	default:
		return false
	}
}

func validTransferIdentity(identity vocabulary.TransferIdentity) bool {
	return identity >= vocabulary.TransferIdentityUnspecified && identity <= vocabulary.TransferIdentityDistinct
}

func validTransferCapabilities(capabilities vocabulary.TransferCapabilities) bool {
	return capabilities >= vocabulary.TransferCapabilitiesUnspecified && capabilities <= vocabulary.TransferCapabilitiesLoseAll
}

// validTransferInputSource admits only exact invocation inputs.  A ValuesVar
// is a transfer source only when it is the operation input tail; result,
// callback, and local Values variables have no caller-owned source Pack.
// AllInputs is reserved for the synthesized opaque operation.
func validTransferInputSource(source vocabulary.InputSource, d operationDraft) bool {
	switch source.Kind {
	case vocabulary.InputSourceValueFormal:
		return uint64(source.Ordinal) < uint64(d.valueFormalCount())
	case vocabulary.InputSourceValuesVar:
		return d.input.tail == vocabulary.ValuesVariable && source.Ordinal == uint32(d.input.varID)
	default:
		return false
	}
}

func retainedCallbackLifecycle(lifecycle vocabulary.CallbackLifecycle) bool {
	return lifecycle >= vocabulary.CallbackRetainedOptionalOnce && lifecycle <= vocabulary.CallbackRetainedRequiredMany
}

func onceCallbackLifecycle(lifecycle vocabulary.CallbackLifecycle) bool {
	switch lifecycle {
	case vocabulary.CallbackSyncOptionalOnce, vocabulary.CallbackSyncRequiredOnce,
		vocabulary.CallbackRetainedOptionalOnce, vocabulary.CallbackRetainedRequiredOnce:
		return true
	default:
		return false
	}
}

func validCallbackReleaseMode(mode vocabulary.CallbackReleaseMode) bool {
	return mode == vocabulary.CallbackReleaseOne || mode == vocabulary.CallbackReleaseAll
}

func (d operationDraft) valueFormalCount() int {
	return len(d.input.types)
}

func validOperationOutcome(kind flowkind.OutcomeKind) bool {
	switch kind {
	case flowkind.OutcomeNormal, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel:
		return true
	default:
		return false
	}
}

func validValuesTail(tail vocabulary.ValuesTail, variable vocabulary.ValuesVar, count uint32, opaque bool) bool {
	switch tail {
	case vocabulary.ValuesClosed:
		return variable == 0
	case vocabulary.ValuesVariable:
		return uint64(variable) < uint64(count)
	case vocabulary.ValuesUnknown:
		return opaque && variable == 0
	default:
		return false
	}
}
