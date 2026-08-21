package compiler

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) deriveRuleOccurrencesFailure() CompileFailure {
	compiler.publication.RuleOccurrences = []programschema.RuleOccurrence{}
	for index, row := range compiler.publication.Occurrences {
		if !programschema.OccurrenceDenseAvailable(row, compiler.publication.OccurrencePoints, compiler.publication.OccurrenceInputs) {
			compiler.publication.RuleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if uint64(index) > uint64(^uint32(0)) {
			compiler.publication.RuleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		ordinal := uint32(index)
		geometry := compiler.occurrenceSpans[occurrenceLookup{kind: row.Kind(), id: row.ID()}]
		finish := geometry.finish
		if len(finish) == 0 {
			var finishOK bool
			finish, finishOK = programschema.OccurrencePointIDs(row, compiler.publication.OccurrencePoints)
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
	compiler.publication.RuleOccurrences = orderPlacementsByDeclaration(compiler.issuance, compiler.publication.RuleOccurrences)
	return CompileFailure{}
}

func orderPlacementsByDeclaration(directory issuance.Directory, rows []programschema.RuleOccurrence) []programschema.RuleOccurrence {
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
	for index := 0; index < directory.Count(); index++ {
		issued, present := directory.At(index)
		if !present {
			return nil
		}
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
