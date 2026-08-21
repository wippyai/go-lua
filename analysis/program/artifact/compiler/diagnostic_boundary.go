package compiler

import (
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/diagnostic"
)

// copyDiagnosticObservationsFailure is the sole root boundary for diagnostic
// construction. The child owns all observation state and returns one canonical
// publication; the root only supplies immutable phase outputs and maps the
// compact child fault to the compiler-wide failure vocabulary.
func (compiler *compiler) copyDiagnosticObservationsFailure() CompileFailure {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || !compiler.key.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	publication, fault := diagnostic.Compile(diagnostic.Input{
		Program:        compiler.input,
		ProgramID:      compiler.key.ProgramID(),
		Values:         compiler.values,
		ValuesMembers:  compiler.valuesMembers,
		Calls:          compiler.calls,
		CallArguments:  compiler.callArguments,
		BodyBoundary:   compiler.bodyBoundary,
		Allocations:    compiler.allocations,
		PointIDsBySite: compiler.pointIDsBySite,
	})
	if fault.Failed() {
		return mapDiagnosticFault(fault)
	}
	compiler.diagnostic = publication
	return CompileFailure{}
}

func mapDiagnosticFault(fault diagnostic.Fault) CompileFailure {
	row, subrow := fault.Row(), fault.Subrow()
	switch fault.Kind() {
	case diagnostic.FaultRouteUnavailable:
		return compileFailure(CompileStageRoutes, CompileRowRoute, row, subrow, CompileReasonRouteUnavailable)
	case diagnostic.FaultRouteGuard:
		return compileFailure(CompileStageRoutes, CompileRowRoute, row, subrow, CompileReasonRouteGuard)
	case diagnostic.FaultStorageRead:
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, row, subrow, CompileReasonOccurrenceStorageRead)
	case diagnostic.FaultCall:
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, row, subrow, CompileReasonOccurrenceCall)
	case diagnostic.FaultDuplicate:
		return compileFailure(CompileStageCanonicalize, CompileRowRoute, row, subrow, CompileReasonRouteGuard)
	case diagnostic.FaultInvalidInput, diagnostic.FaultUnavailable:
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, row, subrow, CompileReasonOccurrenceUnavailable)
	default:
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, row, subrow, CompileReasonOccurrenceUnavailable)
	}
}
