// Package compiler owns the complete Program-to-Artifact compilation
// transaction. All construction state and failures die with CompileDetailed;
// the returned artifact retains only its immutable canonical publication.
package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	calltargetcompile "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/calltarget"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/diagnostic"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

// CompileDetailed compiles one immutable Program under an execution schema and
// issuance directory. It is the sole diagnostic compilation entry point.
func CompileDetailed(input *program.Program, executionSchema programartifact.ExecutionSchemaID, issuance schemaissuance.Plan) (*programartifact.Artifact, CompileFailure) {
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
		issuanceRows: programissuance.NewBuilder(), localTransfer: localtransfer.New(artifactFormat()),
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
		return nil, CompileFailure{construction: failure}
	}
	if failure := transaction.copySubjectAliasFailure(); failure.Available() {
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
	// The subject-liveness join authenticates a Flow subject against the
	// Program owner that mounts it. A concat carries no Flow operator
	// primitive, so its owner is the occurrence issued at its evaluation span
	// by the catalog above; the join therefore reads the occurrence plane and
	// runs after it is complete.
	if failure := transaction.copySubjectLivenessFailure(); failure.Available() {
		return nil, failure
	}
	diagnosticPublication, diagnosticFault := diagnostic.Compile(diagnostic.Input{
		Program:       transaction.input,
		Values:        transaction.publication.Values,
		ValuesMembers: transaction.publication.ValuesMembers,
		Calls:         transaction.publication.Calls,
		CallArguments: transaction.publication.CallArguments,
		BodyBoundary:  transaction.bodyBoundary,
		Allocations:   transaction.allocations,
	})
	if diagnosticFault.Available() {
		return nil, CompileFailure{construction: diagnosticFault}
	}
	transaction.publication.Diagnostic = diagnosticPublication
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
	// The route index stays live through stage installation: a routed stage is
	// placed on the route that reaches it, so placement asks where each route
	// comes from. It is released once, below, when nothing reads it again.
	if failure := transaction.installLocalStagesFailure(); failure.Available() {
		return nil, failure
	}
	// Synthetic stage directories and the post-rewrite route indexes are now
	// fully reflected in canonical points, routes, WTO events, and rule rows.
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
		_, fault := calltargetcompile.Build(calltargetcompile.Input{})
		return CompileFailure{construction: fault}
	}
	rows, fault := calltargetcompile.Build(calltargetcompile.Input{
		Program:     compiler.input,
		Allocations: compiler.allocations,
		Bodies:      compiler.bodyBoundary,
	})
	if fault.Available() {
		return CompileFailure{construction: fault}
	}
	proofs, proofFault := calltargetcompile.ClosureCaptureProofs(calltargetcompile.Input{
		Program: compiler.input, Allocations: compiler.allocations, Bodies: compiler.bodyBoundary,
	}, rows)
	if proofFault.Available() {
		return CompileFailure{construction: proofFault}
	}
	for _, occurrence := range proofs {
		if compiler.issuanceRows == nil || !compiler.issuanceRows.AddClosureProof(occurrence) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	compiler.publication.CallTargets = rows
	return CompileFailure{}
}

// Compile compiles one sealed Program under the supplied execution schema and
// reports whether the immutable artifact was published.
func Compile(input *program.Program, executionSchema programartifact.ExecutionSchemaID, issuance schemaissuance.Plan) (*programartifact.Artifact, bool) {
	result, failure := CompileDetailed(input, executionSchema, issuance)
	return result, result != nil && !failure.Available()
}
