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
	StatusImportOnly       Status = "import_or_stdlib_operational"
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
	ReturnsErrorReturn                 = "returns.ErrorReturn"
	ReturnsReturnLength                = "returns.ReturnLength"
	ReturnsReturnDeepElementOf         = "returns.Return.DeepElementOf"
	ReturnsReturnStringUnpackValue     = "returns.Return.StringUnpackValue"
	ReturnsReturnSelectCaseOfParam     = "returns.Return.SelectCaseOfParam"
	ReturnsReturnSelectResultOfCases   = "returns.Return.SelectResultOfCases"
	ReturnsCorrelatedReturn            = "returns.CorrelatedReturn"

	PostconditionNormalReturnRefinement = "postcondition.NormalReturnRefinement"

	OwnershipBorrow    = "ownership.Borrow"
	OwnershipRetain    = "ownership.Retain"
	OwnershipStore     = "ownership.Store"
	OwnershipSendParam = "ownership.SendParam"
	OwnershipExport    = "ownership.Export"
	OwnershipOpaque    = "ownership.Opaque"
	OwnershipFreeze    = "ownership.Freeze"
	OwnershipBorrowAll = "ownership.BorrowAll"

	IterationIterator = "iteration.Iterator"

	DispatchModuleLoad        = "dispatch.ModuleLoad"
	DispatchTypePredicate     = "dispatch.TypePredicate"
	DispatchVariadicTransform = "dispatch.VariadicTransform"

	MutationMutate       = "mutation.Mutate"
	MutationLengthChange = "mutation.LengthChange"
	MutationTableMutator = "mutation.TableMutator"

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
	ReturnsErrorReturn:                 descriptor(ReturnsErrorReturn, "returns", "ErrorReturn", StatusOperational, "Error return effect is actively lowered end-to-end."),
	ReturnsReturnLength:                descriptor(ReturnsReturnLength, "returns", "ReturnLength", StatusReserved, "Data/codec vocabulary; not actively lowered into return semantics."),
	ReturnsReturnDeepElementOf:         descriptor(ReturnsReturnDeepElementOf, "returns", "Return.DeepElementOf", StatusReserved, "Reserved transform; lowering falls back to declared returns."),
	ReturnsReturnStringUnpackValue:     descriptor(ReturnsReturnStringUnpackValue, "returns", "Return.StringUnpackValue", StatusReservedHighRisk, "Reserved metadata; lowering ignores it, so stdlib must not declare it while inactive."),
	ReturnsReturnSelectCaseOfParam:     descriptor(ReturnsReturnSelectCaseOfParam, "returns", "Return.SelectCaseOfParam", StatusReserved, "Reserved transform; lowering falls back to declared returns."),
	ReturnsReturnSelectResultOfCases:   descriptor(ReturnsReturnSelectResultOfCases, "returns", "Return.SelectResultOfCases", StatusReserved, "Reserved transform; lowering falls back to declared returns."),
	ReturnsCorrelatedReturn:            descriptor(ReturnsCorrelatedReturn, "returns", "CorrelatedReturn", StatusReservedHighRisk, "Reserved metadata; lowering ignores it, so stdlib must not declare it while inactive."),

	PostconditionNormalReturnRefinement: descriptor(PostconditionNormalReturnRefinement, "postcondition", "NormalReturnRefinement", StatusOperational, "Normal-return refinement is actively consumed by postcondition handling."),

	OwnershipBorrow:    descriptor(OwnershipBorrow, "ownership", "Borrow", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipRetain:    descriptor(OwnershipRetain, "ownership", "Retain", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipStore:     descriptor(OwnershipStore, "ownership", "Store", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipSendParam: descriptor(OwnershipSendParam, "ownership", "SendParam", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipExport:    descriptor(OwnershipExport, "ownership", "Export", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipOpaque:    descriptor(OwnershipOpaque, "ownership", "Opaque", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipFreeze:    descriptor(OwnershipFreeze, "ownership", "Freeze", StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipBorrowAll: descriptor(OwnershipBorrowAll, "ownership", "BorrowAll", StatusImportOnly, "Import/stdlib operational vocabulary, but not exported as the same ownership label."),

	IterationIterator: descriptor(IterationIterator, "iteration", "Iterator", StatusImportOnly, "Import/stdlib operational vocabulary, but not exported as the same label."),

	DispatchModuleLoad:        descriptor(DispatchModuleLoad, "dispatch", "ModuleLoad", StatusPartial, "Metadata marker for require-like signatures; operational module rehydration is currently name-bound to require and does not inspect this label."),
	DispatchTypePredicate:     descriptor(DispatchTypePredicate, "dispatch", "TypePredicate", StatusReservedHighRisk, "Reserved metadata; type() narrowing is syntax/factflow based, so stdlib must not declare this while inactive."),
	DispatchVariadicTransform: descriptor(DispatchVariadicTransform, "dispatch", "VariadicTransform", StatusReservedHighRisk, "Reserved metadata; select() lowering ignores this, so stdlib must not declare it while inactive."),

	MutationMutate:       descriptor(MutationMutate, "mutation", "Mutate", StatusPartial, "Operational lowering consumes only Target as a path-invalidation authority; Transform and LengthDelta are metadata until shape/length mutation semantics are implemented."),
	MutationLengthChange: descriptor(MutationLengthChange, "mutation", "LengthChange", StatusPartial, "Operational lowering consumes only Target as a path-invalidation authority; Delta is metadata until length semantics are implemented."),
	MutationTableMutator: descriptor(MutationTableMutator, "mutation", "TableMutator", StatusPartial, "Operational lowering consumes only Target as a path-invalidation authority; Value is metadata until element write semantics are implemented."),

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
