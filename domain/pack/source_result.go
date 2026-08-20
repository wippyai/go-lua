package pack

import "github.com/wippyai/go-lua/analysis/identity"

// sourceResultRow is the immutable, seal-time result for one source-producing
// root. Rows align with Schema roots; non-source roots remain unavailable.
type sourceResultRow struct {
	ready  bool
	source Source
	root   Root
	value  Value
}

// SourceResult is an opaque owner-fenced receipt for one already-constructed
// Pack source value. Hot rules consume it without rebuilding Builder terms,
// scalar endpoints, cases, or relation membership.
type SourceResult struct {
	schema *schema
	index  uint32
}

type sourceOccurrenceRow struct {
	module     identity.ContentID
	occurrence identity.ContentID
	result     uint32
}

type sourceOccurrenceRef struct {
	module     identity.ContentID
	occurrence identity.ContentID
}

func (result SourceResult) valid() bool {
	if result.schema == nil || uint64(result.index) >= uint64(len(result.schema.results)) {
		return false
	}
	row := result.schema.results[result.index]
	return row.ready && row.source.valid() && row.source.schema == result.schema && row.root.valid() && row.root.schema == result.schema && row.root.index == result.index &&
		row.source.root == row.root && row.value.valid() && row.value.owner == result.schema.owner && !row.value.IsBottom() && !row.value.IsTop() && row.value.relation == result.schema.relations[result.index]
}

// SourceResult returns the sole seal-time receipt for this exact Source.
func (schema *Schema) SourceResult(source Source) (SourceResult, bool) {
	if schema == nil || schema.state == nil || !source.valid() || source.schema != schema.state || source.root.schema != schema.state || uint64(source.root.index) >= uint64(len(schema.state.results)) {
		return SourceResult{}, false
	}
	result := SourceResult{schema: schema.state, index: source.root.index}
	row := schema.state.results[source.root.index]
	return result, result.valid() && row.source == source && row.root == source.root
}

// OwnsSourceResult authenticates an exact receipt against this Schema.
func (schema *Schema) OwnsSourceResult(result SourceResult) bool {
	return schema != nil && schema.state != nil && result.schema == schema.state && result.valid()
}

func (result SourceResult) Source() (Source, bool) {
	if !result.valid() {
		return Source{}, false
	}
	return result.schema.results[result.index].source, true
}

func (result SourceResult) Root() (Root, bool) {
	if !result.valid() {
		return Root{}, false
	}
	return result.schema.results[result.index].root, true
}

func (result SourceResult) Value() (Value, bool) {
	if !result.valid() {
		return Value{}, false
	}
	return result.schema.results[result.index].value, true
}

func (schema *Schema) sealSourceResults() bool {
	if schema == nil || schema.state == nil || schema.state.results != nil {
		return false
	}
	results := make([]sourceResultRow, len(schema.state.roots))
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
		results[index] = sourceResultRow{ready: true, source: source, root: root, value: value}
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
		occurrences = append(occurrences, sourceOccurrenceRow{module: module, occurrence: occurrence, result: uint32(index)})
	}
	schema.state.results = results
	schema.state.sourceOccurrences = occurrences
	schema.state.sourceOccurrenceIndex = occurrenceIndex
	for index, row := range results {
		if !row.ready {
			continue
		}
		if !(SourceResult{schema: schema.state, index: uint32(index)}).valid() {
			return false
		}
	}
	return true
}

// SourceResultForMountedOccurrence is the sole direct inverse from a mounted
// Program occurrence to Pack's already sealed source receipt. Mounted owners
// redeem through it instead of retaining a module-scoped directory.
func (schema *Schema) SourceResultForMountedOccurrence(module, occurrence identity.ContentID) (SourceResult, bool) {
	if schema == nil || schema.state == nil || !module.Available() || !occurrence.Available() {
		return SourceResult{}, false
	}
	slot := schema.state.sourceOccurrenceIndex[sourceOccurrenceRef{module: module, occurrence: occurrence}]
	if slot == 0 || uint64(slot) > uint64(len(schema.state.sourceOccurrences)) {
		return SourceResult{}, false
	}
	row := schema.state.sourceOccurrences[slot-1]
	result := SourceResult{schema: schema.state, index: row.result}
	return result, row.module == module && row.occurrence == occurrence && result.valid()
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
