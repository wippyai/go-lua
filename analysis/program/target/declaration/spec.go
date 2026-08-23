package declaration

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Spec is a one-shot authoring container. Consume consumes it on its first
// attempt, including an attempt that fails validation.
type Spec struct {
	// Semantics is the explicit domain implementation of the schema type
	// contract. Target never supplies a default: every sealed target must name
	// the type authority that validates declarations and proves relations.
	Semantics schematype.Semantics
	// Types is the complete authored name-to-type vocabulary for the
	// target. Names are already canonical and qualified at this boundary; the
	// compiler seals them into owner-issued Type handles.
	Types             []vocabulary.QualifiedTypeSpec
	Operations        []vocabulary.OperationSpec
	Protocols         []vocabulary.ProtocolSpec
	InitialRoots      []vocabulary.InitialRootSpec
	InitialEntries    []vocabulary.InitialEntrySpec
	InitialBindings   []vocabulary.InitialBindingSpec
	InitialMetatables []vocabulary.InitialMetatableAttachmentSpec
	consumed          bool
}

// Input is the complete declaration handed to the compiler. Consume transfers
// the slice graph by ownership; the caller must not retain or mutate aliases
// after consuming the Spec. The compiler synchronously freezes that graph
// into its canonical immutable representation before publishing a Contract.
type Input struct {
	Semantics         schematype.Semantics
	Types             []vocabulary.QualifiedTypeSpec
	Operations        []vocabulary.OperationSpec
	Protocols         []vocabulary.ProtocolSpec
	InitialRoots      []vocabulary.InitialRootSpec
	InitialEntries    []vocabulary.InitialEntrySpec
	InitialBindings   []vocabulary.InitialBindingSpec
	InitialMetatables []vocabulary.InitialMetatableAttachmentSpec
}

// Consume destructively transfers one complete declaration to the compiler.
// The transfer is one-shot: a nil or consumed Spec never yields a second
// Input. It deliberately does not clone the nested graph. Ownership of every
// authoring slice moves to the returned Input, and the caller must not retain
// or mutate aliases after this call. The semantic adapter remains an opaque
// domain value; declaration owns only its placement and transfer boundary.
func (spec *Spec) Consume() (Input, error) {
	if spec == nil {
		return Input{}, errors.New("target/declaration: nil spec")
	}
	if spec.consumed {
		return Input{}, errors.New("target/declaration: consumed spec")
	}
	defer func() { *spec = Spec{consumed: true} }()
	return Input{
		Semantics:         spec.Semantics,
		Types:             spec.Types,
		Operations:        spec.Operations,
		Protocols:         spec.Protocols,
		InitialRoots:      spec.InitialRoots,
		InitialEntries:    spec.InitialEntries,
		InitialBindings:   spec.InitialBindings,
		InitialMetatables: spec.InitialMetatables,
	}, nil
}
