package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// staticTypeValueDraft is compiler-only assembly state for one executable
// TypeValue source. Publication converts it to the canonical Program family.
type staticTypeValueDraft struct {
	id        identity.ContentID
	body      identity.ContentID
	reference identity.ContentID
	root      identity.ContentID
	name      string
}

func (row staticTypeValueDraft) Available() bool {
	return row.id.Available() && row.body.Available() && row.reference.Available() && row.root.Available() && row.name != ""
}
func (row staticTypeValueDraft) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row staticTypeValueDraft) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row staticTypeValueDraft) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}
func (row staticTypeValueDraft) RootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.root
}
func (row staticTypeValueDraft) Name() string {
	if !row.Available() {
		return ""
	}
	return row.name
}

func (compiler *compiler) copyStaticRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	compiler.staticTypeValues = make([]staticTypeValueDraft, 0)
	typeValues := compiler.input.Flow().Authored().TypeValues()
	for index := 0; index < typeValues.Count(); index++ {
		source, referenceID, rootID, name, sourceOK := compiler.typeValueCompileRow(index)
		if !sourceOK {
			continue // authored denominator includes dead TypeValue candidates
		}
		if !source.id.Available() || !source.body.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		row := staticTypeValueDraft{id: source.id, body: source.body, reference: referenceID, root: rootID, name: name}
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		compiler.staticTypeValues = append(compiler.staticTypeValues, row)
	}
	return CompileFailure{}
}

// StaticTypeValueCount and StaticTypeValueAt expose executable TypeValue
// source rows without exporting the authored source coordinate.
