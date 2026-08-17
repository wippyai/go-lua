package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// StaticTypeArgumentRow is the closed Program row for one authored call
// type argument.  The argument and target are owner-issued identities; no
// authored Term or Program/Static capability crosses the artifact boundary.
type StaticTypeArgumentRow struct {
	id        identity.ContentID
	call      identity.ContentID
	types     identity.ContentID
	reference identity.ContentID
	index     uint32
}

func (row StaticTypeArgumentRow) Available() bool {
	return row.id.Available() && row.call.Available() && row.types.Available() && row.reference.Available()
}
func (row StaticTypeArgumentRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row StaticTypeArgumentRow) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}
func (row StaticTypeArgumentRow) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}
func (row StaticTypeArgumentRow) TypesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.types
}
func (row StaticTypeArgumentRow) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.index
}

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
	// copyCallRowsFailure is the sole call source stage. Static's ordered
	// type-argument references are already copied into the Artifact-owned
	// CallTypeArgumentRow column there, so this stage only projects that direct
	// column into StaticTypeArgumentRow without reopening Program wrappers.
	compiler.staticTypeArguments = make([]StaticTypeArgumentRow, 0, len(compiler.callTypeArguments))
	for index, argument := range compiler.callTypeArguments {
		row := StaticTypeArgumentRow{id: argument.id, call: argument.call, types: argument.types, reference: argument.reference, index: argument.position}
		if !argument.Available() || !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		compiler.staticTypeArguments = append(compiler.staticTypeArguments, row)
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

// StaticTypeArgumentCount and StaticTypeArgumentAt expose the closed
// Program-owned type-argument formal plane to mounted Static authorities.
func (artifact *Artifact) StaticTypeArgumentCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticTypeArguments)
}
func (artifact *Artifact) StaticTypeArgumentAt(index int) (StaticTypeArgumentRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticTypeArguments) {
		return StaticTypeArgumentRow{}, false
	}
	return artifact.staticTypeArguments[index], true
}

// StaticTypeValueCount and StaticTypeValueAt expose executable TypeValue
// source rows without exporting the authored source coordinate.
func (artifact *Artifact) StaticTypeValueCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticTypeValues)
}
func (artifact *Artifact) StaticTypeValueAt(index int) (StaticTypeValueRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticTypeValues) {
		return StaticTypeValueRow{}, false
	}
	return artifact.staticTypeValues[index], true
}

func (artifact *Artifact) StaticTypeNodeCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticTypeNodes)
}
func (artifact *Artifact) StaticTypeNodeAt(index int) (StaticTypeNodeRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticTypeNodes) {
		return StaticTypeNodeRow{}, false
	}
	return artifact.staticTypeNodes[index], true
}
func (artifact *Artifact) StaticExpressionCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticExpressions)
}
func (artifact *Artifact) StaticExpressionAt(index int) (StaticExpressionRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticExpressions) {
		return StaticExpressionRow{}, false
	}
	return artifact.staticExpressions[index], true
}

func (artifact *Artifact) StaticExpressionByID(id identity.ContentID) (StaticExpressionRow, bool) {
	if artifact == nil || !id.Available() {
		return StaticExpressionRow{}, false
	}
	for _, row := range artifact.staticExpressions {
		if row.id == id {
			return row, true
		}
	}
	return StaticExpressionRow{}, false
}

func (artifact *Artifact) StaticInputCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticInputs)
}
func (artifact *Artifact) StaticInputAt(index int) (StaticInputRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticInputs) {
		return StaticInputRow{}, false
	}
	return artifact.staticInputs[index], true
}
