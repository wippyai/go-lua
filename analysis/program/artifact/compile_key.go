// Package programartifact owns the immutable, reusable analyzer artifact for
// one sealed Program. It retains no Link, engine, schema, runtime, callback,
// raw Term, or domain authority.
package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
)

const (
	// GrammarABIVersion is the cold schema/artifact contract. It is data in
	// every CompileKey, not an ambient package assumption.
	GrammarABIVersion = uint64(5)

	artifactFormat = uint64(33)
	// Region heads are derived from the canonical first member; the former
	// duplicate head/sourceHead scalars no longer enter the artifact identity.
	pointGeometryLawVersion = uint64(2)
	// Attachment relations are emitted directly into the generic occurrence
	// catalog; v2 removes the former retained Site-to-WTO projection from the
	// artifact identity preimage.
	pointAttachmentLawVersion = uint64(2)
	// Dead structural Boundary, StaticTypeArgument, and duplicated Static node
	// field metadata projections no longer survive the compiler.
	compilerLawVersion     = uint64(5)
	operatorLawVersion     = uint64(1)
	substitutionLawVersion = uint64(1)
	summaryLawVersion      = uint64(1)
	wtoLawVersion          = uint64(1)
	routeLawVersion        = uint64(3)
	valuesLawVersion       = uint64(1)
	bodyOutcomeLawVersion  = uint64(4)
	// Function boundaries reference the canonical Body outcome range instead
	// of retaining a second ordered outcome-ID slice.
	functionBoundaryLawVersion = uint64(3)
	// Heap allocation geometry now commits the parent-issued
	// SharesFirstValueCell relation. Keep this generic occurrence law separate
	// from Pack's row law: changing Heap rows must invalidate only the
	// reusable occurrence/artifact identity contract.
	occurrenceLawVersion = uint64(12)
	// v3 records the closed DiagnosticObservation union, including detached
	// unresolved-reference proof and exact branch payload masks.
	diagnosticLawVersion     = uint64(5)
	callRowsLawVersion       = uint64(2)
	callResultRowsLawVersion = uint64(2)

	compileKeyDomain      = "analysis/program-artifact/compile-key"
	artifactIDDomain      = "analysis/program-artifact/artifact"
	grammarIdentityDomain = "analysis/program-artifact/grammar"
)

// ArtifactFormatVersion is the immutable representation version committed by
// every CompileKey and Artifact identity.
const ArtifactFormatVersion = artifactFormat

// GrammarIdentity is the pointer-free cold grammar admitted by the Program
// artifact compiler. It carries only the sealed schema digest and ABI; live
// SchemaBinding authority is joined later by the composition root.
type GrammarIdentity struct {
	schema identity.ContentID
	abi    uint64
	id     identity.ContentID
}

// NewGrammarIdentity constructs the exact cold grammar identity from the
// sealed schema's digest and ABI. A zero digest or foreign ABI is rejected so
// a caller cannot compile under an unavailable or mismatched schema.
func NewGrammarIdentity(schema identity.ContentID, abi uint64) (GrammarIdentity, bool) {
	if !schema.Available() || abi != GrammarABIVersion {
		return GrammarIdentity{}, false
	}
	grammar := GrammarIdentity{schema: schema, abi: abi}
	grammar.id = digest(grammarIdentityDomain, artifactFormat, bytesField(grammar.schema), uintField(grammar.abi))
	return grammar, grammar.Available()
}

func (grammar GrammarIdentity) Available() bool {
	return grammar.schema.Available() && grammar.abi == GrammarABIVersion && grammar.id.Available()
}

func (grammar GrammarIdentity) SchemaDigest() identity.ContentID {
	if !grammar.Available() {
		return identity.ContentID{}
	}
	return grammar.schema
}

func (grammar GrammarIdentity) ABIVersion() uint64 {
	if !grammar.Available() {
		return 0
	}
	return grammar.abi
}

func (grammar GrammarIdentity) ID() identity.ContentID {
	if !grammar.Available() {
		return identity.ContentID{}
	}
	return grammar.id
}

// CompileKey is the complete reusable cold compiler identity. Every law
// version is retained as data and committed by both the key and Artifact ID.
type CompileKey struct {
	program             identity.ContentID
	grammar             GrammarIdentity
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

func NewCompileKey(input *program.Program, grammar GrammarIdentity) (CompileKey, bool) {
	if !input.Available() || !grammar.Available() {
		return CompileKey{}, false
	}
	key := CompileKey{
		program: input.ContentID(), grammar: grammar, format: artifactFormat,
		compilerLaw: compilerLawVersion, operatorLaw: operatorLawVersion,
		substituteLaw: substitutionLawVersion, summaryLaw: summaryLawVersion,
		wtoLaw: wtoLawVersion, routeLaw: routeLawVersion, valuesLaw: valuesLawVersion,
		bodyOutcomeLaw: bodyOutcomeLawVersion, functionBoundaryLaw: functionBoundaryLawVersion, occurrenceLaw: occurrenceLawVersion, diagnosticLaw: diagnosticLawVersion,
		callRowsLaw: callRowsLawVersion,
	}
	key.id = digest(compileKeyDomain, artifactFormat, key.identityFields()...)
	return key, key.Available()
}

func (key CompileKey) identityFields() []field {
	return []field{
		bytesField(key.program), bytesField(key.grammar.ID()), bytesField(key.grammar.SchemaDigest()),
		uintField(key.grammar.ABIVersion()), uintField(key.format), uintField(key.compilerLaw),
		uintField(key.operatorLaw), uintField(key.substituteLaw), uintField(key.summaryLaw),
		uintField(key.wtoLaw), uintField(key.routeLaw), uintField(key.valuesLaw), uintField(key.bodyOutcomeLaw), uintField(key.functionBoundaryLaw), uintField(key.occurrenceLaw), uintField(key.diagnosticLaw),
		uintField(key.callRowsLaw),
	}
}

func (key CompileKey) Available() bool {
	return key.program.Available() && key.grammar.Available() && key.format == artifactFormat &&
		key.compilerLaw == compilerLawVersion && key.operatorLaw == operatorLawVersion &&
		key.substituteLaw == substitutionLawVersion && key.summaryLaw == summaryLawVersion &&
		key.wtoLaw == wtoLawVersion && key.routeLaw == routeLawVersion && key.valuesLaw == valuesLawVersion &&
		key.bodyOutcomeLaw == bodyOutcomeLawVersion && key.functionBoundaryLaw == functionBoundaryLawVersion && key.occurrenceLaw == occurrenceLawVersion &&
		key.diagnosticLaw == diagnosticLawVersion && key.callRowsLaw == callRowsLawVersion && key.id.Available()
}

func (key CompileKey) ProgramID() identity.ContentID {
	if !key.Available() {
		return identity.ContentID{}
	}
	return key.program
}

func (key CompileKey) Grammar() GrammarIdentity {
	if !key.Available() {
		return GrammarIdentity{}
	}
	return key.grammar
}

func (key CompileKey) SchemaDigest() identity.ContentID {
	return key.Grammar().SchemaDigest()
}

func (key CompileKey) ABIVersion() uint64 { return key.Grammar().ABIVersion() }

func (key CompileKey) ID() identity.ContentID {
	if !key.Available() {
		return identity.ContentID{}
	}
	return key.id
}
