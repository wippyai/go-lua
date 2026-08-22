// Package programartifact owns the immutable, reusable analyzer artifact for
// one sealed Program. It retains no Link, engine, schema, runtime, callback,
// raw Term, or domain authority.
package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
)

const (
	// GrammarABIVersion is the cold schema/artifact contract. It is data in
	// every CompileKey, not an ambient package assumption.
	GrammarABIVersion = uint64(6)

	artifactFormat = uint64(34)
	// Issuance is now one sealed schema-driven machine. Rule stages and inputs
	// are exact schema keys, stage identity/transport is declaration data, and
	// the compiler emits final receipts atomically from the generic schedule.
	// Advance both laws so artifacts from the ordinal/mirror representation can
	// never be admitted under the new geometry.
	compilerLawVersion     = uint64(7)
	operatorLawVersion     = uint64(1)
	substitutionLawVersion = uint64(1)
	summaryLawVersion      = uint64(1)
	wtoLawVersion          = uint64(1)
	routeLawVersion        = uint64(3)
	valuesLawVersion       = uint64(1)
	bodyOutcomeLawVersion  = uint64(4)
	// The occurrence vocabulary carries the structural BinaryConcat family, so
	// the same source seals a wider occurrence plane than the previous law
	// admitted. Keep this generic occurrence law separate from Pack's row law:
	// changing Heap rows must invalidate only the reusable
	// occurrence/artifact identity contract.
	occurrenceLawVersion  = uint64(14)
	compileKeyDomain      = "analysis/program-artifact/compile-key"
	artifactIDDomain      = "analysis/program-artifact/artifact"
	executionSchemaDomain = "analysis/program-artifact/execution-schema"
)

// ArtifactFormatVersion is the immutable representation version committed by
// every CompileKey and Artifact identity.
const ArtifactFormatVersion = artifactFormat

// ExecutionSchemaID is the one foreign-consumer admission identity for a
// sealed execution schema. It is deliberately a content identity, rather
// than a wrapper carrying the fields from which it was derived: the cold
// Compilation digest and the order-sensitive Publication schema ID are
// already sealed at the composition root, and the artifact boundary must
// retain only their atomic result.
type ExecutionSchemaID [32]byte

// NewExecutionSchemaID folds exactly the cold meaning, snapshot layout, and
// ABI into the artifact admission identity. Derived publication plans are not
// part of this preimage. A foreign or unavailable ABI fails closed.
func NewExecutionSchemaID(cold, publication identity.ContentID, abi uint64) (ExecutionSchemaID, bool) {
	if !cold.Available() || !publication.Available() || abi != GrammarABIVersion {
		return ExecutionSchemaID{}, false
	}
	id := artifactdigest.Digest(
		executionSchemaDomain,
		artifactFormat,
		artifactdigest.ContentID(cold),
		artifactdigest.ContentID(publication),
		artifactdigest.Uint(abi),
	)
	if !id.Available() {
		return ExecutionSchemaID{}, false
	}
	return ExecutionSchemaID(id), true
}

// Available reports whether the ID names a sealed execution schema.
func (id ExecutionSchemaID) Available() bool { return identity.ContentID(id).Available() }

// ContentID returns the canonical shared identity carried by the artifact
// admission token.
func (id ExecutionSchemaID) ContentID() identity.ContentID {
	if !id.Available() {
		return identity.ContentID{}
	}
	return identity.ContentID(id)
}

// CompileKey is the complete reusable cold compiler identity. Every law
// version is retained as data and committed by both the key and Artifact ID.
type CompileKey struct {
	program             identity.ContentID
	executionSchema     ExecutionSchemaID
	format              uint64
	compilerLaw         uint64
	operatorLaw         uint64
	substituteLaw       uint64
	summaryLaw          uint64
	wtoLaw              uint64
	routeLaw            uint64
	valuesLaw           uint64
	bodyOutcomeLaw      uint64
	functionBoundaryLaw uint64
	occurrenceLaw       uint64
	diagnosticLaw       uint64
	callRowsLaw         uint64
	id                  identity.ContentID
}

func NewCompileKey(input *program.Program, executionSchema ExecutionSchemaID) (CompileKey, bool) {
	if !input.Available() || !executionSchema.Available() {
		return CompileKey{}, false
	}
	key := CompileKey{
		program: input.ContentID(), executionSchema: executionSchema, format: artifactFormat,
		compilerLaw: compilerLawVersion, operatorLaw: operatorLawVersion,
		substituteLaw: substitutionLawVersion, summaryLaw: summaryLawVersion,
		wtoLaw: wtoLawVersion, routeLaw: routeLawVersion, valuesLaw: valuesLawVersion,
		bodyOutcomeLaw: bodyOutcomeLawVersion, functionBoundaryLaw: programschema.FunctionBoundaryLawVersion, occurrenceLaw: occurrenceLawVersion, diagnosticLaw: programdiagnostic.DiagnosticRowsLawVersion,
		callRowsLaw: programschema.CallRowsLawVersion,
	}
	key.id = artifactdigest.Digest(compileKeyDomain, artifactFormat, key.identityFields()...)
	return key, key.Available()
}

func (key CompileKey) identityFields() []artifactdigest.Field {
	return []artifactdigest.Field{
		artifactdigest.ContentID(key.program), artifactdigest.ContentID(key.executionSchema.ContentID()), artifactdigest.Uint(key.format), artifactdigest.Uint(key.compilerLaw),
		artifactdigest.Uint(key.operatorLaw), artifactdigest.Uint(key.substituteLaw), artifactdigest.Uint(key.summaryLaw),
		artifactdigest.Uint(key.wtoLaw), artifactdigest.Uint(key.routeLaw), artifactdigest.Uint(key.valuesLaw), artifactdigest.Uint(key.bodyOutcomeLaw), artifactdigest.Uint(key.functionBoundaryLaw), artifactdigest.Uint(key.occurrenceLaw), artifactdigest.Uint(key.diagnosticLaw),
		artifactdigest.Uint(key.callRowsLaw),
	}
}

func (key CompileKey) Available() bool {
	return key.program.Available() && key.executionSchema.Available() && key.format == artifactFormat &&
		key.compilerLaw == compilerLawVersion && key.operatorLaw == operatorLawVersion &&
		key.substituteLaw == substitutionLawVersion && key.summaryLaw == summaryLawVersion &&
		key.wtoLaw == wtoLawVersion && key.routeLaw == routeLawVersion && key.valuesLaw == valuesLawVersion &&
		key.bodyOutcomeLaw == bodyOutcomeLawVersion && key.functionBoundaryLaw == programschema.FunctionBoundaryLawVersion && key.occurrenceLaw == occurrenceLawVersion &&
		key.diagnosticLaw == programdiagnostic.DiagnosticRowsLawVersion && key.callRowsLaw == programschema.CallRowsLawVersion && key.id.Available()
}

func (key CompileKey) ProgramID() identity.ContentID {
	if !key.Available() {
		return identity.ContentID{}
	}
	return key.program
}

func (key CompileKey) ExecutionSchemaID() ExecutionSchemaID {
	if !key.Available() {
		return ExecutionSchemaID{}
	}
	return key.executionSchema
}

func (key CompileKey) ID() identity.ContentID {
	if !key.Available() {
		return identity.ContentID{}
	}
	return key.id
}
