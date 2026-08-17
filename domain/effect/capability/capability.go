// Package capability owns the central classification of effect vocabulary.
//
// This package is deliberately metadata-only and imports nothing from the
// effect packages. The dependency runs the other way: each leaf label package
// imports this one to state, beside the label type, which capability the type
// is. That direction is what makes the ID authored once — the catalog names it,
// the label points at the name, and nothing re-derives the pairing from a Go
// type.
package capability

import (
	"sort"
	"strings"
)

type Status string

const (
	StatusOperational    Status = "operational"
	StatusImportOrStdlib Status = "import_or_stdlib_operational"
	StatusPartial        Status = "partial"

	// StatusManifestValidated classifies vocabulary that a manifest declares and
	// the module boundary validates, while no lowering turns it into analysis
	// facts. It is distinct from the reserved tiers, which bar a label from
	// manifests and from stdlib signatures.
	StatusManifestValidated Status = "manifest_validated"

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
	ReturnsReturnSameAs:                descriptor(ReturnsReturnSameAs, StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnElementOf:             descriptor(ReturnsReturnElementOf, StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnOptionalElementOf:     descriptor(ReturnsReturnOptionalElementOf, StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnCallbackReturn:        descriptor(ReturnsReturnCallbackReturn, StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnArrayOfCallbackReturn: descriptor(ReturnsReturnArrayOfCallbackReturn, StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnTypeProjection:        descriptor(ReturnsReturnTypeProjection, StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsReturnConditionalType:       descriptor(ReturnsReturnConditionalType, StatusOperational, "Return transform is actively lowered end-to-end."),
	ReturnsErrorReturn:                 descriptor(ReturnsErrorReturn, StatusOperational, "Error return effect is actively lowered end-to-end."),
	ReturnsReturnLength:                descriptor(ReturnsReturnLength, StatusReserved, "Data/codec vocabulary; not actively lowered into return semantics."),
	ReturnsCorrelatedReturn:            descriptor(ReturnsCorrelatedReturn, StatusReservedHighRisk, "Reserved metadata; lowering ignores it, so stdlib must not declare it while inactive."),

	PostconditionNormalReturnRefinement: descriptor(PostconditionNormalReturnRefinement, StatusOperational, "Normal-return refinement is actively consumed by postcondition handling."),

	OwnershipBorrow:    descriptor(OwnershipBorrow, StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipRetain:    descriptor(OwnershipRetain, StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipStore:     descriptor(OwnershipStore, StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipSend:      descriptor(OwnershipSend, StatusImportOrStdlib, "Import/stdlib operational suffix-send vocabulary; analyzed exports use per-param SendParam instead."),
	OwnershipSendParam: descriptor(OwnershipSendParam, StatusOperational, "Ownership label is actively consumed end-to-end."),
	OwnershipExport:    descriptor(OwnershipExport, StatusReserved, "Reserved manifest vocabulary; no placement decision consumes it."),
	OwnershipOpaque:    descriptor(OwnershipOpaque, StatusReservedHighRisk, "Reserved manifest vocabulary; no placement decision consumes it."),
	OwnershipFreeze:    descriptor(OwnershipFreeze, StatusReserved, "Reserved manifest vocabulary; table.freeze has no effect-row placement consumer."),
	OwnershipBorrowAll: descriptor(OwnershipBorrowAll, StatusImportOrStdlib, "Import/stdlib operational vocabulary, but not exported as the same ownership label."),

	IterationIterator: descriptor(IterationIterator, StatusImportOrStdlib, "Import/stdlib operational vocabulary, but not exported as the same label."),

	DispatchModuleLoad:   descriptor(DispatchModuleLoad, StatusImportOrStdlib, "Import/stdlib operational capability: module identity and provider boundaries bind through this label."),
	MutationMutate:       descriptor(MutationMutate, StatusPartial, "Operational lowering consumes only Target as a path-invalidation authority; Transform and LengthDelta are metadata until shape/length mutation semantics are implemented."),
	MutationLengthChange: descriptor(MutationLengthChange, StatusPartial, "Operational lowering consumes Target as a path-invalidation authority and positive Delta as a length-floor proof; negative Delta remains metadata until precise shrink semantics are implemented."),
	MutationTableMutator: descriptor(MutationTableMutator, StatusOperational, "Operational lowering invalidates the target and publishes indexed element evidence from Value end-to-end."),

	LifecycleAcquire:    descriptor(LifecycleAcquire, StatusManifestValidated, "Acquire state and obligation are carried in signature manifests and validated against the declared typestate FSM; no lowering consumes it."),
	LifecycleTransition: descriptor(LifecycleTransition, StatusManifestValidated, "Transition endpoints are carried in signature manifests and validated against the declared typestate FSM; no lowering consumes it."),
	LifecycleEscape:     descriptor(LifecycleEscape, StatusManifestValidated, "Escaping protocol is carried in signature manifests and validated against the declared typestate FSM; no lowering consumes it."),

	ControlThrow: descriptor(ControlThrow, StatusReservedHighRisk, "Reserved metadata; control throw lowering is inactive, so stdlib must not declare it while behavior is represented by Never/postconditions/module-load."),
	ControlIO:    descriptor(ControlIO, StatusReservedHighRisk, "Reserved metadata; IO policy/enforcement is inactive, so stdlib must not declare it while inactive."),
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

// descriptor derives the descriptor's parts from the ID it is keyed by. The ID
// is the audited name and its shape is family-dot-symbol, so restating either
// half beside it would be the same name written twice.
func descriptor(id string, status Status, rationale string) Descriptor {
	family, symbol, _ := strings.Cut(id, ".")
	return Descriptor{
		ID:        id,
		Family:    family,
		Symbol:    symbol,
		Status:    status,
		Rationale: rationale,
	}
}
