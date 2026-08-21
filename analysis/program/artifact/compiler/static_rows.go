package compiler

import (
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) copyStaticRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	compiler.publication.StaticTypeValues = make([]programschema.StaticTypeValue, 0)
	typeValues := compiler.input.Flow().Authored().TypeValues()
	for index := 0; index < typeValues.Count(); index++ {
		source, referenceID, rootID, name, sourceOK := compiler.typeValueCompileRow(index)
		if !sourceOK {
			continue // authored denominator includes dead TypeValue candidates
		}
		if !source.id.Available() || !source.body.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		row, rowOK := programschema.NewStaticTypeValue(source.id, source.body, referenceID, rootID, name)
		if !rowOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		compiler.publication.StaticTypeValues = append(compiler.publication.StaticTypeValues, row)
	}
	return CompileFailure{}
}

// StaticTypeValueCount and StaticTypeValueAt expose executable TypeValue
// source rows without exporting the authored source coordinate.
