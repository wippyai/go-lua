// Package capability owns the central classification of effect vocabulary.
//
// This package is deliberately metadata-only. Keep it free of imports from the
// effect subpackages so leaf labels can depend on the root effect package
// without creating import cycles.
package capability

import "sort"

type Status string

const (
	StatusOperational      Status = "operational"
	StatusImportOrStdlib   Status = "import_or_stdlib_operational"
	StatusPartial          Status = "partial"
	StatusReserved         Status = "reserved"
	StatusReservedHighRisk Status = "reserved_high_risk"
)

type Descriptor struct {
	ID        string
	Family    string
	Symbol    string
	Status    Status
	Rationale string
}

const (
	ReturnsReturnSameAs                = "returns.Return.SameAs"
	ReturnsReturnElementOf             = "returns.Return.ElementOf"
	ReturnsReturnOptionalElementOf     = "returns.Return.OptionalElementOf"
	ReturnsReturnCallbackReturn        = "returns.Return.CallbackReturn"
	ReturnsReturnArrayOfCallbackReturn = "returns.Return.ArrayOfCallbackReturn"
	ReturnsReturnTypeProjection        = "returns.Return.TypeProjection"
	ReturnsReturnConditionalType       = "returns.Return.ConditionalType"
	ReturnsErrorReturn                 = "returns.ErrorReturn"
	ReturnsReturnLength                = "returns.ReturnLength"
	ReturnsCorrelatedReturn            = "returns.CorrelatedReturn"

	PostconditionNormalReturnRefinement = "postcondition.NormalReturnRefinement"

	OwnershipBorrow    = "ownership.Borrow"
	OwnershipRetain    = "ownership.Retain"
	OwnershipStore     = "ownership.Store"
	OwnershipSend      = "ownership.Send"
	OwnershipSendParam = "ownership.SendParam"
	OwnershipExport    = "ownership.Export"
	OwnershipOpaque    = "ownership.Opaque"
	OwnershipFreeze    = "ownership.Freeze"
	OwnershipBorrowAll = "ownership.BorrowAll"

	IterationIterator = "iteration.Iterator"

	DispatchModuleLoad = "dispatch.ModuleLoad"

	MutationMutate       = "mutation.Mutate"
	MutationLengthChange = "mutation.LengthChange"
	MutationTableMutator = "mutation.TableMutator"

	LifecycleAcquire    = "lifecycle.Acquire"
	LifecycleTransition = "lifecycle.Transition"
	LifecycleEscape     = "lifecycle.Escape"

	ControlThrow = "control.Throw"
	ControlIO    = "control.IO"
)

var descriptors = map[string]Descriptor{
	ReturnsReturnSameAs:                descriptor(ReturnsReturnSameAs, "returns", "Return.SameAs", StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnElementOf:             descriptor(ReturnsReturnElementOf, "returns", "Return.ElementOf", StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnOptionalElementOf:     descriptor(ReturnsReturnOptionalElementOf, "returns", "Return.OptionalElementOf", StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnCallbackReturn:        descriptor(ReturnsReturnCallbackReturn, "returns", "Return.CallbackReturn", StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnArrayOfCallbackReturn: descriptor(ReturnsReturnArrayOfCallbackReturn, "returns", "Return.ArrayOfCallbackReturn", StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnTypeProjection:        descriptor(ReturnsReturnTypeProjection, "returns", "Return.TypeProjection", StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnConditionalType:       descriptor(ReturnsReturnConditionalType, "returns", "Return.ConditionalType", StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsErrorReturn:                 descriptor(ReturnsErrorReturn, "returns", "ErrorReturn", StatusOperational, "Error return effect is actively lowered end-to-end."),
	ReturnsReturnLength:                descriptor(ReturnsReturnLength, "returns", "ReturnLength", StatusReserved, "Data/codec vocabulary; not actively lowered into return semantics."),
	ReturnsCorrelatedReturn:            descriptor(ReturnsCorrelatedReturn, "returns", "CorrelatedReturn", StatusReservedHighRisk, "Reserved metadata; lowering ignores it, so stdlib must not declare it while inactive."),

	PostconditionNormalReturnRefinement: descriptor(PostconditionNormalReturnRefinement, "postcondition", "NormalReturnRefinement", StatusOperational, "Normal-return refinement is actively consumed by postcondition handling."),

	OwnershipBorrow:    descriptor(OwnershipBorrow, "ownership", "Borrow", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipRetain:    descriptor(OwnershipRetain, "ownership", "Retain", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipStore:     descriptor(OwnershipStore, "ownership", "Store", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipSend:      descriptor(OwnershipSend, "ownership", "Send", StatusImportOrStdlib, "Import/stdlib operational suffix-send vocabulary; analyzed exports use per-param SendParam instead."),
	OwnershipSendParam: descriptor(OwnershipSendParam, "ownership", "SendParam", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipExport:    descriptor(OwnershipExport, "ownership", "Export", StatusReserved, "Reserved manifest vocabulary; no placement decision consumes it."),
	OwnershipOpaque:    descriptor(OwnershipOpaque, "ownership", "Opaque", StatusReservedHighRisk, "Reserved manifest vocabulary; no placement decision consumes it."),
	OwnershipFreeze:    descriptor(OwnershipFreeze, "ownership", "Freeze", StatusReserved, "Reserved manifest vocabulary; table.freeze has no effect-row placement consumer."),
	OwnershipBorrowAll: descriptor(OwnershipBorrowAll, "ownership", "BorrowAll", StatusImportOrStdlib, "Import/stdlib operational vocabulary, but not exported as the same ownership label."),

	IterationIterator: descriptor(IterationIterator, "iteration", "Iterator", StatusImportOrStdlib, "Import/stdlib operational vocabulary, but not exported as the same label."),

	DispatchModuleLoad:   descriptor(DispatchModuleLoad, "dispatch", "ModuleLoad", StatusImportOrStdlib, "Import/stdlib operational capability: module identity and provider boundaries bind through this label."),
	MutationMutate:       descriptor(MutationMutate, "mutation", "Mutate", StatusPartial, "Operational lowering consumes only Target as a path-invalidation authority; Transform and LengthDelta are metadata until shape/length mutation semantics are implemented."),
	MutationLengthChange: descriptor(MutationLengthChange, "mutation", "LengthChange", StatusPartial, "Operational lowering consumes Target as a path-invalidation authority and positive Delta as a length-floor proof; negative Delta remains metadata until precise shrink semantics are implemented."),
	MutationTableMutator: descriptor(MutationTableMutator, "mutation", "TableMutator", StatusOperational, "Operational lowering invalidates the target and publishes indexed element evidence from Value end-to-end."),

	LifecycleAcquire:    descriptor(LifecycleAcquire, "lifecycle", "Acquire", StatusOperational, "Lifecycle acquire is lowered into canonical typestate facts."),
	LifecycleTransition: descriptor(LifecycleTransition, "lifecycle", "Transition", StatusOperational, "Lifecycle transition is lowered into canonical typestate facts."),
	LifecycleEscape:     descriptor(LifecycleEscape, "lifecycle", "Escape", StatusOperational, "Lifecycle escape is lowered into canonical typestate facts."),

	ControlThrow: descriptor(ControlThrow, "control", "Throw", StatusReservedHighRisk, "Reserved metadata; control throw lowering is inactive, so stdlib must not declare it while behavior is represented by Never/postconditions/module-load."),
	ControlIO:    descriptor(ControlIO, "control", "IO", StatusReservedHighRisk, "Reserved metadata; IO policy/enforcement is inactive, so stdlib must not declare it while inactive."),
}

func All() []Descriptor {
	all := make([]Descriptor, 0, len(descriptors))
	for _, d := range descriptors {
		all = append(all, d)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})
	return all
}

func Lookup(id string) (Descriptor, bool) {
	d, ok := descriptors[id]
	return d, ok
}

func descriptor(id, family, symbol string, status Status, rationale string) Descriptor {
	return Descriptor{
		ID:        id,
		Family:    family,
		Symbol:    symbol,
		Status:    status,
		Rationale: rationale,
	}
}
