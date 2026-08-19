// Package programartifact owns the immutable, reusable analyzer artifact for
// one sealed Program. It retains no Link, engine, schema, runtime, callback,
// raw Term, or domain authority.
package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

const (
	// GrammarABIVersion is the cold schema/artifact contract. It is data in
	// every CompileKey, not an ambient package assumption.
	GrammarABIVersion = uint64(5)

	artifactFormat             = uint64(33)
	pointGeometryLawVersion    = uint64(1)
	pointAttachmentLawVersion  = uint64(1)
	compilerLawVersion         = uint64(2)
	operatorLawVersion         = uint64(1)
	substitutionLawVersion     = uint64(1)
	summaryLawVersion          = uint64(1)
	wtoLawVersion              = uint64(1)
	routeLawVersion            = uint64(3)
	valuesLawVersion           = uint64(1)
	bodyOutcomeLawVersion      = uint64(4)
	functionBoundaryLawVersion = uint64(2)
	// Heap allocation geometry now commits the parent-issued
	// SharesFirstValueCell relation. Keep this generic occurrence law separate
	// from Pack's row law: changing Heap rows must invalidate only the
	// reusable occurrence/artifact identity contract.
	occurrenceLawVersion = uint64(12)
	// v3 records the closed DiagnosticObservation union, including detached
	// unresolved-reference proof and exact branch payload masks.
	diagnosticLawVersion = uint64(5)
	callRowsLawVersion   = uint64(2)

	compileKeyDomain      = "analysis/program-artifact/compile-key"
	artifactIDDomain      = "analysis/program-artifact/artifact"
	grammarIdentityDomain = "analysis/program-artifact/grammar"
)

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

// CompileDetailed is the exact diagnostic lane for the immutable Program
// artifact compiler. The composition root supplies the neutral cold
// GrammarIdentity and the sealed issuance directory; no domain or schema
// authority crosses this package.
func CompileDetailed(input *program.Program, grammar GrammarIdentity, issuance IssuanceDirectory) (*Artifact, CompileFailure) {
	if !input.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	if !grammar.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonGrammarUnavailable)
	}
	key, ok := NewCompileKey(input, grammar)
	if !ok {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonCompileKeyUnavailable)
	}
	counts := input.CountRows()
	if !counts.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	transaction := compiler{
		input: input, key: key, counts: counts, issuance: issuance, points: make(map[identity.ContentID]struct{}), pointGeometry: make(map[identity.ContentID]Point),
		occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry), routeOccurrences: make(map[identity.ContentID]identity.ContentID), predecessorStages: make(map[identity.ContentID]identity.ContentID), localStages: make(map[identity.ContentID]identity.ContentID), computationStages: make(map[identity.ContentID][]computationStage), callStages: make(map[identity.ContentID]callStageSet),
		pointIDsBySite:     make(map[identity.ContentID][]identity.ContentID),
		environmentByRoute: make(map[identity.ContentID]EnvironmentEdge), environmentRouteDuplicates: make(map[identity.ContentID]struct{}),
		diagnosticObservationByID: make(map[identity.ContentID]int),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyBodiesAndOutcomesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyFunctionBoundariesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyAllocationRowsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyCallTargetsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyCallRowsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyBoundaryRowsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyHeapGeometryFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyLocalWTOFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.emitRoutesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.canonicalizePointDecisionsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyOccurrenceCatalogFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyDiagnosticObservationsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyStaticRowsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyStaticGraphFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.deriveArithmeticSummariesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.deriveRuleOccurrencesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.installLocalStagesFailure(); failure.Available() {
		return nil, failure
	}
	if transaction.ruleOccurrences == nil {
		return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	if failure := transaction.finalizeFailure(); failure.Available() {
		return nil, failure
	}
	artifact, failure := transaction.sealArtifact()
	if failure.Available() {
		return nil, failure
	}
	return artifact, CompileFailure{}
}

// Compile compiles one sealed Program under the supplied cold grammar and
// reports whether the immutable artifact was published.
const compileSizeHintCap = 1 << 20

func compileSizeHint(counts denominator.CountRows) int {
	if !counts.Available() {
		return 0
	}
	var total uint64
	for index := 0; index < counts.Count(); index++ {
		row, ok := counts.At(index)
		if !ok {
			return 0
		}
		if row.Count() > uint64(^uint(0))-total {
			return compileSizeHintCap
		}
		total += row.Count()
	}
	if total > compileSizeHintCap {
		return compileSizeHintCap
	}
	return int(total)
}

func Compile(input *program.Program, grammar GrammarIdentity, issuance IssuanceDirectory) (*Artifact, bool) {
	artifact, failure := CompileDetailed(input, grammar, issuance)
	return artifact, artifact != nil && !failure.Available()
}
