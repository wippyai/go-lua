package program

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

const programSemanticSourcePublicationCount = 57

// SemanticSourcePublications returns the canonical Program-owned semantic
// source measure as detached claims. Program retains no duplicate claim
// vector; each claim is read from an immutable typed owner column and sealed
// into a caller-owned value for this query only.
func (program *Program) SemanticSourcePublications(schema semanticsource.ProgramSchema) []semanticsource.Publication {
	counts, definitions, ok := program.semanticSourceMeasures(schema)
	if !ok {
		return nil
	}
	rows := make([]semanticsource.Publication, 0, len(definitions))
	for index, definition := range definitions {
		row, err := semanticsource.SealPublication(definition, counts[index])
		if err != nil {
			return nil
		}
		rows = append(rows, row)
	}
	return rows
}

func (program *Program) semanticSourceMeasures(schema semanticsource.ProgramSchema) ([]int, []semanticsource.RelationDef, bool) {
	if schema == nil || schema.Count() == 0 || !schema.SchemaDigest().Available() {
		return nil, nil, false
	}
	counts, err := deriveProgramSemanticSourceCounts(program, schema)
	if err != nil {
		return nil, nil, false
	}
	definitions, ok := programSemanticSourceDefinitions(schema)
	if !ok || len(counts) != len(definitions) {
		return nil, nil, false
	}
	return counts[:], definitions, true
}

// deriveProgramSemanticSourceCounts is the root composition cut. It reads
// each owner's immutable view, maps the generated schema rows to the owner
// cardinalities, and retains no second row representation on Program.
func deriveProgramSemanticSourceCounts(program *Program, schema semanticsource.ProgramSchema) ([programSemanticSourcePublicationCount]int, error) {
	var result [programSemanticSourcePublicationCount]int
	if schema == nil || schema.Count() == 0 || !schema.SchemaDigest().Available() {
		return result, errors.New("unavailable semantic-source schema")
	}
	if program == nil || program.source == nil || program.flow == nil || program.static == nil || program.module == nil || !program.id.Available() {
		return result, errors.New("unavailable Program owner quartet")
	}

	sourceID := program.source.Cold().ContentID()
	flowID := program.flow.ContentID()
	staticID := program.static.ContentID()
	moduleID := program.module.View().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return result, errors.New("unavailable Program child identity")
	}
	provenance := program.flow.View().Provenance()
	if provenance.Source != sourceID || provenance.Flow != flowID || provenance.Static != staticID || provenance.Module != moduleID {
		return result, errors.New("Program Flow provenance mismatch")
	}
	rootID, err := rootContentID(sourceID, flowID, staticID, moduleID)
	if err != nil || rootID != program.id {
		return result, errors.New("Program owner identity mismatch")
	}

	sourceCounts, err := programSourceCounts(program.source.View())
	if err != nil {
		return result, err
	}
	flowCounts, err := programFlowCounts(program.flow.View())
	if err != nil {
		return result, err
	}
	staticCounts, err := programStaticCounts(program.static.View())
	if err != nil {
		return result, err
	}
	moduleCounts, err := programModuleCounts(program.module.View())
	if err != nil {
		return result, err
	}

	definitions, ok := programSemanticSourceDefinitions(schema)
	if !ok {
		return result, errors.New("invalid generated Program semantic-source schema")
	}
	for index, definition := range definitions {
		count, ok := programOwnerCount(definition.Token(), sourceCounts, flowCounts, staticCounts, moduleCounts)
		if !ok || count < 0 || !programSemanticSourceCountFits(count) {
			return result, fmt.Errorf("invalid semantic-source count at schema ordinal %d", index)
		}
		// SealPublication validates the generated definition/count contract at
		// the root cut without retaining a second row representation.
		if _, err := semanticsource.SealPublication(definition, count); err != nil {
			return result, err
		}
		result[index] = count
	}
	return result, nil
}
