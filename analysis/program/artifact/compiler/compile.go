// Package compiler owns the complete Program-to-Artifact compilation
// transaction. All construction state and failures die with CompileDetailed;
// the returned artifact retains only its immutable canonical publication.
package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	calltargetcompile "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/calltarget"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	stageplan "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/stage"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
)

// CompileDetailed compiles one immutable Program under an execution schema and
// issuance directory. It is the sole diagnostic compilation entry point.
func CompileDetailed(input *program.Program, executionSchema programartifact.ExecutionSchemaID, issuance issuance.Directory) (*programartifact.Artifact, CompileFailure) {
	if !input.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	if !executionSchema.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonGrammarUnavailable)
	}
	key, ok := programartifact.NewCompileKey(input, executionSchema)
	if !ok {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonCompileKeyUnavailable)
	}
	counts := input.CountRows()
	if !counts.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	transaction := compiler{
		input: input, key: key, counts: counts, issuance: issuance, pointGeometry: make(map[identity.ContentID]pointDraft),
		occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry), stages: stageplan.New(artifactFormat()), localTransfer: localtransfer.New(artifactFormat()),
		pointIDsBySite:     make(map[identity.ContentID][]identity.ContentID),
		environmentByRoute: make(map[identity.ContentID]environmentRouteIndex),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyBodyBoundaryFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyStorageCellLifetimesFailure(); failure.Available() {
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
	if failure := transaction.copyModuleRowsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copySubjectLivenessFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copySubjectAliasFailure(); failure.Available() {
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
	// Diagnostic construction is the final Site-to-point consumer. Its child
	// package owns all diagnostic-only indexes and caches; only the immutable
	// publication remains on the transaction for sealing.
	transaction.pointIDsBySite = nil
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
	// Rule issuance has consumed occurrence span geometry and the original
	// route index generation. Stage installation rebuilds its own route index
	// after it rewrites environment sources.
	transaction.occurrenceSpans = nil
	transaction.environmentByRoute = nil
	if failure := transaction.installLocalStagesFailure(); failure.Available() {
		return nil, failure
	}
	// Synthetic stage directories and the post-rewrite route indexes are now
	// fully reflected in canonical points, routes, WTO events, and rule rows.
	transaction.stages = nil
	transaction.environmentByRoute = nil
	if transaction.publication.RuleOccurrences == nil {
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

// copyCallTargetsFailure is the transaction boundary for the child-owned
// closure-to-callable join. Only fault translation and canonical row storage
// remain in the compiler shell.
func (compiler *compiler) copyCallTargetsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	rows, fault := calltargetcompile.Build(calltargetcompile.Input{
		Program:     compiler.input,
		Allocations: compiler.allocations,
		Bodies:      compiler.bodyBoundary,
	})
	if fault.Failed() {
		reason := CompileReasonBodyUnavailable
		if fault.Reason() == calltargetcompile.ReasonDuplicate {
			reason = CompileReasonBodyDuplicate
		}
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, fault.Row(), fault.Subrow(), reason)
	}
	compiler.publication.CallTargets = rows
	return CompileFailure{}
}

// Compile compiles one sealed Program under the supplied execution schema and
// reports whether the immutable artifact was published.
func Compile(input *program.Program, executionSchema programartifact.ExecutionSchemaID, issuance issuance.Directory) (*programartifact.Artifact, bool) {
	result, failure := CompileDetailed(input, executionSchema, issuance)
	return result, result != nil && !failure.Available()
}
