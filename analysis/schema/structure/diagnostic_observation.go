package structure

import "github.com/wippyai/go-lua/analysis/schema"

// DiagnosticObservationKind is the canonical identity of one reusable
// diagnostic-observation population. The ordinal is the identity carried by
// an artifact observation and is deliberately kept in this neutral schema
// package so Program and its consumers do not depend on a domain composition.
type DiagnosticObservationKind uint8

const (
	DiagnosticObservationInvalid DiagnosticObservationKind = iota
	DiagnosticObservationBranchCondition
	DiagnosticObservationTypeReferenceUnresolved
	DiagnosticObservationValueReferenceUnresolved
)

type diagnosticObservationDefinition struct {
	kind     DiagnosticObservationKind
	key      schema.Key
	spelling string
}

// This is the one authored observation inventory. Its order and ordinals are
// ABI-bearing: artifact observation ContentIDs frame the kind as this ordinal,
// so changing either the constants or these definitions changes the identity
// preimage and is not a local refactor.
var diagnosticObservationDefinitions = [...]diagnosticObservationDefinition{
	{DiagnosticObservationBranchCondition, "observation/branch-condition", "branch-condition"},
	{DiagnosticObservationTypeReferenceUnresolved, "observation/type-reference-unresolved", "type-reference-unresolved"},
	{DiagnosticObservationValueReferenceUnresolved, "observation/value-reference-unresolved", "value-reference-unresolved"},
}

// Available reports whether kind belongs to the canonical observation
// vocabulary.
func (kind DiagnosticObservationKind) Available() bool {
	return kind >= DiagnosticObservationBranchCondition && kind <= DiagnosticObservationValueReferenceUnresolved
}

// Ordinal returns the dense structural-vocabulary ordinal carried by kind.
func (kind DiagnosticObservationKind) Ordinal() uint16 {
	if !kind.Available() {
		return 0
	}
	return uint16(kind)
}

// Key returns the authored schema identity of kind.
func (kind DiagnosticObservationKind) Key() schema.Key {
	if !kind.Available() {
		return ""
	}
	return diagnosticObservationDefinitions[int(kind)-1].key
}

// ID returns the stable structural entry identity of kind.
func (kind DiagnosticObservationKind) ID() schema.EntryID {
	return schema.NewEntryID(schema.SurfaceKindStructure, kind.Key())
}

// Spelling returns the rendered name declared for kind.
func (kind DiagnosticObservationKind) Spelling() string {
	if !kind.Available() {
		return ""
	}
	return diagnosticObservationDefinitions[int(kind)-1].spelling
}

// DiagnosticObservationSpecs returns the canonical structural declarations
// for all diagnostic-observation populations. The returned slice is detached
// so callers cannot mutate the inventory owned by this package.
func DiagnosticObservationSpecs() []Spec {
	specs := make([]Spec, 0, len(diagnosticObservationDefinitions))
	for _, definition := range diagnosticObservationDefinitions {
		specs = append(specs, Spec{
			Key:      definition.key,
			Category: CategoryDiagnosticObservation,
			Ordinal:  definition.kind.Ordinal(),
			Spelling: definition.spelling,
			Accepted: true,
		})
	}
	return specs
}

// DiagnosticObservationEntry projects the canonical kind through a sealed
// structural vocabulary. It is the neutral projection boundary for consumers
// that need the declaration's entry identity or spelling.
func DiagnosticObservationEntry(table Table, kind DiagnosticObservationKind) (*Entry, bool) {
	if !kind.Available() {
		return nil, false
	}
	return table.At(CategoryDiagnosticObservation, kind.Ordinal())
}
