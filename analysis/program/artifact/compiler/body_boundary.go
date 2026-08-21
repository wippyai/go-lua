package compiler

import "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/bodyboundary"

// copyBodyBoundaryFailure is the single parent handoff for the complete body,
// outcome, and callable-boundary plane. Construction and its indexes belong to
// bodyboundary; the compiler retains only the child Bundle until publication.
func (compiler *compiler) copyBodyBoundaryFailure() CompileFailure {
	if compiler == nil || compiler.input == nil {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	bundle, fault := bodyboundary.Build(bodyboundary.Input{
		Program:        compiler.input,
		ProgramID:      compiler.key.ProgramID(),
		Values:         compiler.publication.Values,
		PointIDsBySite: compiler.pointIDsBySite,
	})
	if !fault.Failed() {
		compiler.bodyBoundary = bundle
		return CompileFailure{}
	}
	return mapBodyBoundaryFailure(fault)
}

func mapBodyBoundaryFailure(fault bodyboundary.Fault) CompileFailure {
	row, subrow := fault.Row(), fault.Subrow()
	switch fault.Reason() {
	case bodyboundary.ReasonForeign:
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, row, subrow, CompileReasonBodyForeign)
	case bodyboundary.ReasonIdentity:
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, row, subrow, CompileReasonBodyIdentity)
	case bodyboundary.ReasonDuplicate:
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, row, subrow, CompileReasonBodyDuplicate)
	case bodyboundary.ReasonRange:
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, row, subrow, CompileReasonBodyRange)
	case bodyboundary.ReasonOutcomeUnavailable:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeUnavailable)
	case bodyboundary.ReasonOutcomeAttachment:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeAttachment)
	case bodyboundary.ReasonOutcomeShape:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeShape)
	case bodyboundary.ReasonOutcomeForeign:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeForeign)
	case bodyboundary.ReasonOutcomeIdentity:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeIdentity)
	case bodyboundary.ReasonOutcomeDuplicate:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeDuplicate)
	case bodyboundary.ReasonOutcomeKind:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeKind)
	case bodyboundary.ReasonOutcomeTarget:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeTarget)
	case bodyboundary.ReasonOutcomePropagation:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomePropagation)
	case bodyboundary.ReasonOutcomeReference:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeReference)
	case bodyboundary.ReasonOutcomeRange:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeRange)
	case bodyboundary.ReasonOutcomeReturn:
		return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, row, subrow, CompileReasonOutcomeReturn)
	case bodyboundary.ReasonReturnValueUnavailable:
		return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, row, subrow, CompileReasonReturnValueUnavailable)
	case bodyboundary.ReasonReturnValueReference:
		return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, row, subrow, CompileReasonReturnValueReference)
	default:
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, row, subrow, CompileReasonBodyUnavailable)
	}
}
