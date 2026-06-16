package label

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
)

// IDFor returns the audited capability ID for a concrete effect label.
func IDFor(label effect.Label) (string, bool) {
	switch l := effect.NormalizeLabel(label).(type) {
	case returns.Return:
		return IDForReturnTransform(l.Transform)
	case returns.ErrorReturn:
		return capability.ReturnsErrorReturn, true
	case returns.ReturnLength:
		return capability.ReturnsReturnLength, true
	case returns.CorrelatedReturn:
		return capability.ReturnsCorrelatedReturn, true
	case postcondition.NormalReturnRefinement:
		return capability.PostconditionNormalReturnRefinement, true
	case ownership.Borrow:
		return capability.OwnershipBorrow, true
	case ownership.Retain:
		return capability.OwnershipRetain, true
	case ownership.Store:
		return capability.OwnershipStore, true
	case ownership.Send:
		return capability.OwnershipSend, true
	case ownership.SendParam:
		return capability.OwnershipSendParam, true
	case ownership.Export:
		return capability.OwnershipExport, true
	case ownership.Opaque:
		return capability.OwnershipOpaque, true
	case ownership.Freeze:
		return capability.OwnershipFreeze, true
	case ownership.BorrowAll:
		return capability.OwnershipBorrowAll, true
	case iteration.Iterator:
		return capability.IterationIterator, true
	case dispatch.ModuleLoad:
		return capability.DispatchModuleLoad, true
	case dispatch.TypePredicate:
		return capability.DispatchTypePredicate, true
	case dispatch.VariadicTransform:
		return capability.DispatchVariadicTransform, true
	case mutation.Mutate:
		return capability.MutationMutate, true
	case mutation.LengthChange:
		return capability.MutationLengthChange, true
	case mutation.TableMutator:
		return capability.MutationTableMutator, true
	case control.Throw:
		return capability.ControlThrow, true
	case control.IO:
		return capability.ControlIO, true
	default:
		return "", false
	}
}

// DescriptorFor returns the audited capability descriptor for a concrete effect
// label.
func DescriptorFor(label effect.Label) (capability.Descriptor, bool) {
	id, ok := IDFor(label)
	if !ok {
		return capability.Descriptor{}, false
	}
	return capability.Lookup(id)
}

// IDForReturnTransform returns the audited capability ID for a return transform.
func IDForReturnTransform(transform returns.ReturnType) (string, bool) {
	switch transform.(type) {
	case returns.SameAs:
		return capability.ReturnsReturnSameAs, true
	case *returns.SameAs:
		return capability.ReturnsReturnSameAs, true
	case returns.ElementOf:
		return capability.ReturnsReturnElementOf, true
	case *returns.ElementOf:
		return capability.ReturnsReturnElementOf, true
	case returns.OptionalElementOf:
		return capability.ReturnsReturnOptionalElementOf, true
	case *returns.OptionalElementOf:
		return capability.ReturnsReturnOptionalElementOf, true
	case returns.CallbackReturn:
		return capability.ReturnsReturnCallbackReturn, true
	case *returns.CallbackReturn:
		return capability.ReturnsReturnCallbackReturn, true
	case returns.ArrayOfCallbackReturn:
		return capability.ReturnsReturnArrayOfCallbackReturn, true
	case *returns.ArrayOfCallbackReturn:
		return capability.ReturnsReturnArrayOfCallbackReturn, true
	case returns.TypeProjection:
		return capability.ReturnsReturnTypeProjection, true
	case *returns.TypeProjection:
		return capability.ReturnsReturnTypeProjection, true
	case returns.DeepElementOf:
		return capability.ReturnsReturnDeepElementOf, true
	case *returns.DeepElementOf:
		return capability.ReturnsReturnDeepElementOf, true
	case returns.StringUnpackValue:
		return capability.ReturnsReturnStringUnpackValue, true
	case *returns.StringUnpackValue:
		return capability.ReturnsReturnStringUnpackValue, true
	case returns.SelectCaseOfParam:
		return capability.ReturnsReturnSelectCaseOfParam, true
	case *returns.SelectCaseOfParam:
		return capability.ReturnsReturnSelectCaseOfParam, true
	case returns.SelectResultOfCases:
		return capability.ReturnsReturnSelectResultOfCases, true
	case *returns.SelectResultOfCases:
		return capability.ReturnsReturnSelectResultOfCases, true
	default:
		return "", false
	}
}

// DescriptorForReturnTransform returns the audited capability descriptor for a
// return transform.
func DescriptorForReturnTransform(transform returns.ReturnType) (capability.Descriptor, bool) {
	id, ok := IDForReturnTransform(transform)
	if !ok {
		return capability.Descriptor{}, false
	}
	return capability.Lookup(id)
}
