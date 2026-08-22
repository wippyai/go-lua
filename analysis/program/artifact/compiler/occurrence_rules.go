package compiler

import (
	issuanceexecutor "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// deriveRuleOccurrencesFailure executes the sealed generic issuance machine
// and publishes only atomic final emissions. No requirement, form, input, or
// stage meaning is switched on in the compiler shell.
func (compiler *compiler) deriveRuleOccurrencesFailure() CompileFailure {
	if compiler == nil || compiler.issuanceRows == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for index, row := range compiler.publication.Occurrences {
		if !programschema.OccurrenceDenseAvailable(row, compiler.publication.OccurrencePoints, compiler.publication.OccurrenceInputs) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	rows, rowsOK := compiler.issuanceRows.Seal(compiler.issuance.Table(), &compiler.publication)
	if !rowsOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	requests, evaluated := issuanceexecutor.Evaluate(compiler.issuance, rows)
	if !evaluated {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	schedule, scheduled := issuanceexecutor.BuildSchedule(artifactFormat(), compiler.issuance, requests)
	if !scheduled {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	compiler.publication.RuleOccurrences = make([]programschema.RuleOccurrence, 0, schedule.EmissionCount())
	for index := 0; index < schedule.EmissionCount(); index++ {
		emission, emissionOK := schedule.EmissionAt(index)
		if !emissionOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		request := emission.Request()
		subscription := request.Subscription()
		input, _ := emission.InputPoint()
		native, nativeOK := emission.Native()
		route, _ := request.Route()
		if !nativeOK || !compiler.appendRuleOccurrence(
			subscription.Rule(), subscription.Writes(), request.Occurrence(),
			emission.Point(), input, request.Stage().Key(), request.Input().Declaration().Key(), route, native,
		) {
			compiler.publication.RuleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	compiler.issuanceSchedule = schedule
	compiler.issuanceRows = nil
	if compiler.publication.RuleOccurrences == nil {
		compiler.publication.RuleOccurrences = []programschema.RuleOccurrence{}
	}
	return CompileFailure{}
}
