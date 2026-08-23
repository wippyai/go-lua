package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	issuanceexecutor "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
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
		inputs := make([]identity.ContentID, emission.InputPointCount())
		for inputIndex := range inputs {
			input, inputOK := emission.InputPointAt(inputIndex)
			if !inputOK {
				compiler.publication.RuleOccurrences = nil
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, inputIndex, CompileReasonOccurrenceUnavailable)
			}
			inputs[inputIndex] = input
		}
		native, nativeOK := emission.Native()
		route, _ := request.Route()
		inputSpec := programissuance.InputNone
		if request.InputCount() != 0 {
			input, inputOK := request.InputAt(0)
			inputDeclaration := input.Declaration()
			if !inputOK || inputDeclaration == nil {
				compiler.publication.RuleOccurrences = nil
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			inputSpec = inputDeclaration.Key()
		}
		if !nativeOK || !compiler.appendRuleOccurrenceVector(
			subscription.Rule(), subscription.Writes(), request.Occurrence(),
			emission.Point(), inputs, request.Stage().Key(), inputSpec, route, native,
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
