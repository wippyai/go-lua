package pack

import "github.com/wippyai/go-lua/analysis/identity"

// SourceValue returns the sealed value for an exact Pack Source row.
// It returns the existing Value directly; no owner wrapper is created for the
// read.
func (schema *Schema) SourceValue(source Source) (Value, bool) {
	if schema == nil || schema.state == nil || !source.valid() || source.schema != schema.state {
		return Value{}, false
	}
	root := source.root
	if uint64(root.index) >= uint64(len(schema.state.sourceValues)) {
		return Value{}, false
	}
	value := schema.state.sourceValues[root.index]
	if !schema.Admit(root, value) || value.IsBottom() || value.IsTop() {
		return Value{}, false
	}
	return value, true
}

// SourceForMountedOccurrence resolves the existing Pack Source descriptor
// for one mounted Program occurrence. The inverse is a sealed Schema index,
// not a per-rule catalogue.
func (schema *Schema) SourceForMountedOccurrence(module, occurrence identity.ContentID) (Source, bool) {
	if schema == nil || schema.state == nil || !module.Available() || !occurrence.Available() {
		return Source{}, false
	}
	valuesIndex, valuesFound := schema.state.artifactValues[artifactValuesKey{module: module, values: occurrence}]
	callIndex, callFound := schema.state.artifactCalls[artifactCallKey{module: module, call: occurrence}]
	if valuesFound == callFound {
		return Source{}, false
	}
	var rootIndex uint32
	if valuesFound {
		if uint64(valuesIndex) >= uint64(len(schema.state.values)) {
			return Source{}, false
		}
		row := schema.state.values[valuesIndex]
		if row.moduleKey != module || row.occurrenceID != occurrence {
			return Source{}, false
		}
		rootIndex = row.root
	} else {
		if uint64(callIndex) >= uint64(len(schema.state.calls)) {
			return Source{}, false
		}
		row := schema.state.calls[callIndex]
		if row.moduleKey != module || row.occurrenceID != occurrence {
			return Source{}, false
		}
		rootIndex = row.root
	}
	issuedModule, issuedOccurrence, identityOK := schema.state.mountedSourceIdentity(rootIndex)
	if !identityOK || issuedModule != module || issuedOccurrence != occurrence {
		return Source{}, false
	}
	root := Root{schema: schema.state, index: rootIndex}
	source, sourceOK := schema.Source(root)
	if !sourceOK {
		return Source{}, false
	}
	_, valueOK := schema.SourceValue(source)
	return source, valueOK
}

func (schema *Schema) sealSourceValues() bool {
	if schema == nil || schema.state == nil || schema.state.sourceValues != nil {
		return false
	}
	values := make([]Value, len(schema.state.roots))
	seenOccurrences := make(map[struct{ module, occurrence identity.ContentID }]struct{})
	for index := range schema.state.roots {
		root := Root{schema: schema.state, index: uint32(index)}
		source, sourceOK := schema.Source(root)
		if !sourceOK {
			continue
		}
		if source.Count() != 1 {
			return false
		}
		item, itemOK := source.At(0)
		builder, builderOK := schema.Builder(root)
		if !itemOK || !builderOK {
			return false
		}
		term, termOK := sealedSourceTerm(builder, item)
		port, portOK := item.Port()
		equation, equationOK := builder.Pack(port, term)
		caseValue, caseOK := builder.Case(equation)
		value, valueOK := builder.Value(caseValue)
		if !termOK || !portOK || !equationOK || !caseOK || !valueOK || !schema.Admit(root, value) || value.IsBottom() || value.IsTop() {
			return false
		}
		values[index] = value
		module, occurrence, identityOK := schema.state.mountedSourceIdentity(uint32(index))
		if !identityOK {
			return false
		}
		ref := struct{ module, occurrence identity.ContentID }{module: module, occurrence: occurrence}
		if _, duplicate := seenOccurrences[ref]; duplicate {
			return false
		}
		seenOccurrences[ref] = struct{}{}
	}
	schema.state.sourceValues = values
	for index, value := range values {
		if !value.valid() {
			continue
		}
		root := Root{schema: schema.state, index: uint32(index)}
		source, sourceOK := schema.Source(root)
		sealed, valueOK := schema.SourceValue(source)
		if !sourceOK || !valueOK || !schema.Admit(root, sealed) || !sameValueRepresentation(sealed, value) {
			return false
		}
	}
	return true
}

func (state *schema) mountedSourceIdentity(rootIndex uint32) (identity.ContentID, identity.ContentID, bool) {
	if state == nil || uint64(rootIndex) >= uint64(len(state.roots)) {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	root := state.roots[rootIndex]
	var module, occurrence identity.ContentID
	switch root.kind {
	case rootValues:
		if uint64(root.sourceIndex) >= uint64(len(state.values)) {
			return identity.ContentID{}, identity.ContentID{}, false
		}
		row := state.values[root.sourceIndex]
		if row.root != rootIndex {
			return identity.ContentID{}, identity.ContentID{}, false
		}
		module, occurrence = row.moduleKey, row.occurrenceID
	case rootCall:
		if uint64(root.sourceIndex) >= uint64(len(state.calls)) {
			return identity.ContentID{}, identity.ContentID{}, false
		}
		row := state.calls[root.sourceIndex]
		if row.root != rootIndex {
			return identity.ContentID{}, identity.ContentID{}, false
		}
		module, occurrence = row.moduleKey, row.occurrenceID
	default:
		return identity.ContentID{}, identity.ContentID{}, false
	}
	return module, occurrence, module.Available() && occurrence.Available()
}

func sealedSourceTerm(builder Builder, item SourceItem) (Term, bool) {
	fixed := make([]Scalar, item.FixedCount())
	for index := range fixed {
		endpoint, endpointOK := item.FixedAt(index)
		if !endpointOK {
			return Term{}, false
		}
		fixed[index], endpointOK = builder.Endpoint(endpoint)
		if !endpointOK {
			return Term{}, false
		}
	}
	tail, offset, open := item.Tail()
	if !open {
		return builder.Closed(fixed...)
	}
	free, freeOK := builder.FreeTail(tail)
	if !freeOK {
		return Term{}, false
	}
	rest, tailOK := builder.Tail(free, offset)
	if !tailOK {
		return Term{}, false
	}
	return builder.Open(fixed, rest, nil)
}
