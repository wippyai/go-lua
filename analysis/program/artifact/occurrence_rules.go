package artifact

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) deriveRuleOccurrencesFailure() CompileFailure {
	compiler.ruleOccurrences = []programschema.RuleOccurrence{}
	for index, row := range compiler.occurrences {
		if !occurrenceDenseAvailable(row, compiler.occurrencePoints, compiler.occurrenceInputs) {
			compiler.ruleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if uint64(index) > uint64(^uint32(0)) {
			compiler.ruleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		ordinal := uint32(index)
		geometry := compiler.occurrenceSpans[occurrenceLookup{kind: row.Kind(), id: row.ID()}]
		finish := geometry.finish
		if len(finish) == 0 {
			var finishOK bool
			finish, finishOK = occurrencePointIDs(row, compiler.occurrencePoints)
			if !finishOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		placements, decided := compiler.matching(row)
		if !decided {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		for _, placement := range placements {
			if !compiler.applyIssuance(row, ordinal, geometry, finish, placement) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	compiler.ruleOccurrences = orderPlacementsByDeclaration(compiler.issuance, compiler.ruleOccurrences)
	return CompileFailure{}
}

func orderPlacementsByDeclaration(directory IssuanceDirectory, rows []programschema.RuleOccurrence) []programschema.RuleOccurrence {
	if len(rows) == 0 {
		return rows
	}
	byKey := make(map[schema.Key][]programschema.RuleOccurrence, directory.Count())
	for _, row := range rows {
		key := row.Key()
		byKey[key] = append(byKey[key], row)
	}
	ordered := make([]programschema.RuleOccurrence, 0, len(rows))
	seen := make(map[schema.Key]struct{}, directory.Count())
	for _, issued := range directory.placements {
		if !issued.Key.Available() {
			continue
		}
		if _, already := seen[issued.Key]; already {
			continue
		}
		seen[issued.Key] = struct{}{}
		ordered = append(ordered, byKey[issued.Key]...)
		delete(byKey, issued.Key)
	}
	leftovers := make([]schema.Key, 0, len(byKey))
	for key := range byKey {
		leftovers = append(leftovers, key)
	}
	sort.Slice(leftovers, func(left, right int) bool { return leftovers[left] < leftovers[right] })
	for _, key := range leftovers {
		ordered = append(ordered, byKey[key]...)
	}
	return ordered
}
