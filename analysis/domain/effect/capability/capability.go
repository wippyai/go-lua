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
	ReturnsReturnStringUnpackValue:     descriptor(ReturnsReturnStringUnpackValue, "returns", "Return.StringUnpackValue", StatusReservedHighRisk, "Declared by string.unpack stdlib data, but lowering ignores it and declared Any becomes the fallback."),
	ReturnsReturnSelectCaseOfParam:     descriptor(ReturnsReturnSelectCaseOfParam, "returns", "Return.SelectCaseOfParam", StatusReserved, "Reserved transform; lowering falls back to declared returns."),
	ReturnsReturnSelectResultOfCases:   descriptor(ReturnsReturnSelectResultOfCases, "returns", "Return.SelectResultOfCases", StatusReserved, "Reserved transform; lowering falls back to declared returns."),
	ReturnsCorrelatedReturn:            descriptor(ReturnsCorrelatedReturn, "returns", "CorrelatedReturn", StatusReserved, "Data/codec vocabulary; not actively lowered into return semantics."),

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

	DispatchModuleLoad:        descriptor(DispatchModuleLoad, "dispatch", "ModuleLoad", StatusPartial, "Require provider hardcodes module name instead of honoring a full payload."),
	DispatchTypePredicate:     descriptor(DispatchTypePredicate, "dispatch", "TypePredicate", StatusReserved, "Reserved dispatch vocabulary without active lowering semantics."),
	DispatchVariadicTransform: descriptor(DispatchVariadicTransform, "dispatch", "VariadicTransform", StatusReserved, "Reserved dispatch vocabulary without active lowering semantics."),

	MutationMutate:       descriptor(MutationMutate, "mutation", "Mutate", StatusPartial, "Mutation invalidation is active, but payload semantics are only partially consumed."),
	MutationLengthChange: descriptor(MutationLengthChange, "mutation", "LengthChange", StatusPartial, "Length mutation vocabulary is present, but semantics are only partially consumed."),
	MutationTableMutator: descriptor(MutationTableMutator, "mutation", "TableMutator", StatusPartial, "Currently drives invalidation only; payloads are mostly ignored."),

	ControlThrow: descriptor(ControlThrow, "control", "Throw", StatusReserved, "Control vocabulary is codec/row-only and has no active lowering semantics."),
	ControlIO:    descriptor(ControlIO, "control", "IO", StatusReserved, "Control vocabulary is codec/row-only and has no active lowering semantics."),
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
