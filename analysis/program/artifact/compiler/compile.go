// Package compiler owns the complete Program-to-Artifact compilation
// transaction. All construction state and failures die with CompileDetailed;
// the returned artifact retains only its immutable canonical publication.
package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// CompileDetailed compiles one immutable Program under a cold grammar and
// issuance directory. It is the sole diagnostic compilation entry point.
func CompileDetailed(input *program.Program, grammar programartifact.GrammarIdentity, issuance IssuanceDirectory) (*programartifact.Artifact, CompileFailure) {
	if !input.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	if !grammar.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonGrammarUnavailable)
	}
	key, ok := programartifact.NewCompileKey(input, grammar)
	if !ok {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonCompileKeyUnavailable)
	}
	counts := input.CountRows()
	if !counts.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	transaction := compiler{
		input: input, key: key, counts: counts, issuance: issuance, pointGeometry: make(map[identity.ContentID]pointDraft),
		occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry), predecessorStages: make(map[identity.ContentID]identity.ContentID), localStages: make(map[identity.ContentID]identity.ContentID), computationStages: make(map[identity.ContentID][]computationStage), callStages: make(map[identity.ContentID]callStageSet),
		pointIDsBySite:     make(map[identity.ContentID][]identity.ContentID),
		environmentByRoute: make(map[identity.ContentID]environmentEdgeDraft), environmentRouteDuplicates: make(map[identity.ContentID]struct{}),
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
func Compile(input *program.Program, grammar programartifact.GrammarIdentity, issuance IssuanceDirectory) (*programartifact.Artifact, bool) {
	result, failure := CompileDetailed(input, grammar, issuance)
	return result, result != nil && !failure.Available()
}
