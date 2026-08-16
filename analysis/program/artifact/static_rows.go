package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// StaticTypeArgumentRow is the closed Program receipt for one authored call
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

// StaticTypeValueRow is the closed Program receipt for one executable
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
	compiler.staticTypeArguments = make([]StaticTypeArgumentRow, 0)
	for callIndex := 0; callIndex < compiler.input.CallCount(); callIndex++ {
		call, callOK := compiler.input.CallAt(callIndex)
		if !callOK || !compiler.input.OwnsCallOccurrence(call) {
			continue
		}
		arguments, argumentsOK := call.TypeArguments()
		if !argumentsOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		for argumentIndex := 0; argumentIndex < arguments.Count(); argumentIndex++ {
			argument, argumentOK := arguments.At(argumentIndex)
			argumentID, idOK := argument.ContextID(), argumentOK
			referenceID, referenceOK := argument.StaticTypeReferenceID()
			if !argumentOK || !compiler.input.OwnsCallTypeArgument(argument) || !idOK || !referenceOK || !call.ContextID().Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, argumentIndex, CompileReasonOccurrenceUnavailable)
			}
			typesID, typesOK := arguments.ContextID(), arguments.ContextID().Available()
			row := StaticTypeArgumentRow{id: argumentID, call: call.ContextID(), types: typesID, reference: referenceID, index: uint32(argumentIndex)}
			if !typesOK || !row.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, argumentIndex, CompileReasonOccurrenceUnavailable)
			}
			compiler.staticTypeArguments = append(compiler.staticTypeArguments, row)
		}
	}
	compiler.staticTypeValues = make([]StaticTypeValueRow, 0)
	for index := 0; index < compiler.input.TypeValueSourceCount(); index++ {
		source, sourceOK := compiler.input.TypeValueSourceAt(index)
		if !sourceOK {
			continue // authored denominator includes dead TypeValue candidates
		}
		if !compiler.input.OwnsValueSourceOccurrence(source) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		body, bodyOK := source.Body()
		id, idOK := source.ContextID(), source.ContextID().Available()
		referenceID, referenceOK := source.StaticTypeReferenceID()
		rootID, rootOK := source.StaticTypeValueRootID()
		name, nameOK := source.StaticTypeValueName()
		if !bodyOK || !idOK || !referenceOK || !rootOK || !nameOK || !body.PathID().Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		row := StaticTypeValueRow{id: id, body: body.PathID(), reference: referenceID, root: rootID, name: name}
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		compiler.staticTypeValues = append(compiler.staticTypeValues, row)
	}
	return CompileFailure{}
}
