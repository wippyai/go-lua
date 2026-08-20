package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// StaticExpressionRow is the artifact compiler's construction vocabulary for
// one authored static expression. The sealed Artifact reads the canonical
// programschema.StaticExpression family and recreates this narrow row view
// only for legacy artifact-local callers.
type StaticExpressionRow struct {
	id, reference, owner identity.ContentID
}

func (row StaticExpressionRow) Available() bool {
	return row.id.Available() && row.reference.Available() && row.owner.Available()
}

func (row StaticExpressionRow) ID() identity.ContentID          { return row.id }
func (row StaticExpressionRow) ReferenceID() identity.ContentID { return row.reference }
func (row StaticExpressionRow) Owner() identity.ContentID       { return row.owner }

// StaticTypeValueRow is the closed Program row for one executable
// TypeValue source. Its BodyPath and occurrence identity are sufficient for
// mounted substitution; Static decides the semantic class and runtime
// disposition after admission.
type StaticTypeValueRow struct {
	id        identity.ContentID
	body      identity.ContentID
	reference identity.ContentID
	root      identity.ContentID
	name      string
}

func (row StaticTypeValueRow) Available() bool {
	return row.id.Available() && row.body.Available() && row.reference.Available() && row.root.Available() && row.name != ""
}
func (row StaticTypeValueRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row StaticTypeValueRow) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row StaticTypeValueRow) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}
func (row StaticTypeValueRow) RootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.root
}
func (row StaticTypeValueRow) Name() string {
	if !row.Available() {
		return ""
	}
	return row.name
}

func (compiler *compiler) copyStaticRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	compiler.staticTypeValues = make([]StaticTypeValueRow, 0)
	typeValues := compiler.input.Flow().Authored().TypeValues()
	for index := 0; index < typeValues.Count(); index++ {
		source, referenceID, rootID, name, sourceOK := compiler.typeValueCompileRow(index)
		if !sourceOK {
			continue // authored denominator includes dead TypeValue candidates
		}
		if !source.id.Available() || !source.body.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		row := StaticTypeValueRow{id: source.id, body: source.body, reference: referenceID, root: rootID, name: name}
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		compiler.staticTypeValues = append(compiler.staticTypeValues, row)
	}
	return CompileFailure{}
}

// StaticTypeValueCount and StaticTypeValueAt expose executable TypeValue
// source rows without exporting the authored source coordinate.
func (artifact *Artifact) StaticTypeValueCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, programschema.StaticTypeValueFamily())
	if !published {
		return 0
	}
	return count
}
func (artifact *Artifact) StaticTypeValueAt(index int) (StaticTypeValueRow, bool) {
	if !artifact.Available() {
		return StaticTypeValueRow{}, false
	}
	return artifact.staticTypeValueRowAt(index)
}

// staticTypeValueRowAt reads one authored type-value binding out of the
// sealed publication. The row is flat there, so the read is a change of
// vocabulary and no plane is retained beside the publication.
func (artifact *Artifact) staticTypeValueRowAt(index int) (StaticTypeValueRow, bool) {
	sealed, held := coldRow(artifact, programschema.StaticTypeValueFamily(), index)
	if !held {
		return StaticTypeValueRow{}, false
	}
	row := StaticTypeValueRow{id: sealed.ID(), body: sealed.BodyPathID(), reference: sealed.ReferenceID(), root: sealed.RootID(), name: sealed.Name()}
	return row, row.Available()
}

func (artifact *Artifact) StaticExpressionCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, programschema.StaticExpressionFamily())
	if !published {
		return 0
	}
	return count
}
func (artifact *Artifact) StaticExpressionAt(index int) (StaticExpressionRow, bool) {
	if !artifact.Available() {
		return StaticExpressionRow{}, false
	}
	return artifact.staticExpressionRowAt(index)
}

// staticExpressionRowAt reads one authored type expression out of the sealed
// publication. The row is flat there, so the read is a change of vocabulary
// and no plane is retained beside the publication.
func (artifact *Artifact) staticExpressionRowAt(index int) (StaticExpressionRow, bool) {
	sealed, held := coldRow(artifact, programschema.StaticExpressionFamily(), index)
	if !held {
		return StaticExpressionRow{}, false
	}
	row := StaticExpressionRow{id: sealed.ID(), reference: sealed.ReferenceID(), owner: sealed.Owner()}
	return row, row.Available()
}

func (artifact *Artifact) StaticExpressionByID(id identity.ContentID) (StaticExpressionRow, bool) {
	if artifact == nil || !id.Available() {
		return StaticExpressionRow{}, false
	}
	count, published := coldCount(artifact, programschema.StaticExpressionFamily())
	if !published {
		return StaticExpressionRow{}, false
	}
	for index := 0; index < count; index++ {
		row, held := artifact.staticExpressionRowAt(index)
		if held && row.id == id {
			return row, true
		}
	}
	return StaticExpressionRow{}, false
}
