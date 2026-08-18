package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Spec is a one-shot authoring container. Seal consumes it on its first
// attempt, including an attempt that fails validation.
type Spec struct {
	// Semantics is the explicit domain implementation of the schema type
	// contract. Target never supplies a default: every sealed target must name
	// the type authority that validates declarations and proves relations.
	Semantics         schematype.Semantics
	Operations        []vocabulary.OperationSpec
	Protocols         []vocabulary.ProtocolSpec
	InitialRoots      []vocabulary.InitialRootSpec
	InitialEntries    []vocabulary.InitialEntrySpec
	InitialBindings   []vocabulary.InitialBindingSpec
	InitialMetatables []vocabulary.InitialMetatableAttachmentSpec
	consumed          bool
}
