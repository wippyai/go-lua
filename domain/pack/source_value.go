package pack

import "github.com/wippyai/go-lua/analysis/identity"

type sourceOccurrenceRow struct {
	module     identity.ContentID
	occurrence identity.ContentID
	root       uint32
}

type sourceOccurrenceRef struct {
	module     identity.ContentID
	occurrence identity.ContentID
}

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
	slot := schema.state.sourceOccurrenceIndex[sourceOccurrenceRef{module: module, occurrence: occurrence}]
	if slot == 0 || uint64(slot) > uint64(len(schema.state.sourceOccurrences)) {
		return Source{}, false
	}
	row := schema.state.sourceOccurrences[slot-1]
	if row.module != module || row.occurrence != occurrence || uint64(row.root) >= uint64(len(schema.state.roots)) {
		return Source{}, false
	}
	root := Root{schema: schema.state, index: row.root}
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
	occurrences := make([]sourceOccurrenceRow, 0)
	occurrenceIndex := make(map[sourceOccurrenceRef]uint32)
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
		rootRow := schema.state.roots[index]
		var module, occurrence identity.ContentID
		switch rootRow.kind {
		case rootValues:
			if uint64(rootRow.sourceIndex) >= uint64(len(schema.state.values)) {
				return false
			}
			row := schema.state.values[rootRow.sourceIndex]
			module, occurrence = row.moduleKey, row.occurrenceID
		case rootCall:
			if uint64(rootRow.sourceIndex) >= uint64(len(schema.state.calls)) {
				return false
			}
			row := schema.state.calls[rootRow.sourceIndex]
			module, occurrence = row.moduleKey, row.occurrenceID
		default:
			return false
		}
		if !module.Available() || !occurrence.Available() {
			return false
		}
		ref := sourceOccurrenceRef{module: module, occurrence: occurrence}
		if _, duplicate := occurrenceIndex[ref]; duplicate {
			return false
		}
		occurrenceIndex[ref] = uint32(len(occurrences) + 1)
		occurrences = append(occurrences, sourceOccurrenceRow{module: module, occurrence: occurrence, root: uint32(index)})
	}
	schema.state.sourceValues = values
	schema.state.sourceOccurrences = occurrences
	schema.state.sourceOccurrenceIndex = occurrenceIndex
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
