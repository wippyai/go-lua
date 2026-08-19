package artifact

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
)

func (compiler *compiler) deriveRuleOccurrencesFailure() CompileFailure {
	compiler.ruleOccurrences = []RuleOccurrence{}
	for index, row := range compiler.occurrences {
		if uint64(index) > uint64(^uint32(0)) {
			compiler.ruleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		ordinal := uint32(index)
		geometry := compiler.occurrenceSpans[occurrenceLookup{kind: row.kind, id: row.id}]
		finish := geometry.finish
		if len(finish) == 0 {
			finish = row.points
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

func orderPlacementsByDeclaration(directory IssuanceDirectory, rows []RuleOccurrence) []RuleOccurrence {
	if len(rows) == 0 {
		return rows
	}
	byKey := make(map[schema.Key][]RuleOccurrence, len(directory))
	for _, row := range rows {
		byKey[row.key] = append(byKey[row.key], row)
	}
	ordered := make([]RuleOccurrence, 0, len(rows))
	seen := make(map[schema.Key]struct{}, len(directory))
	for _, issued := range directory {
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
