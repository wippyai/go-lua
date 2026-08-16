package index

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
)

type rawPayloadKind uint8

const (
	rawPayloadInvalid rawPayloadKind = iota
	rawPayloadFixed
	rawPayloadTail
	rawPayloadNil
	rawPayloadInitial
)

// rawPayload is a cold descriptor for one existing Heap RawPayloadTag. Its
// source is Pack's mounted semantic selector; neither is recurrent fact state.
type rawPayload struct {
	kind        rawPayloadKind
	payload     pack.Payload
	sourceStart uint32
	sourceCount uint32
}

type rawSourceTag uint64

type rawPayloadSource struct {
	payload heapdomain.RawPayloadTag
	source  pack.SemanticSource
}

type rawSource struct {
	coordinate valuedomain.Coordinate
}

// rawCatalog is the one immutable Topology/Link-scoped Pack/Value projection
// shared by both raw operation directions. Heap remains the authority for
// candidate geometry and payload tags; this catalog contains only the Pack
// payload/source rows needed by typed selection callbacks.
type rawCatalog struct {
	payloads        []rawPayload
	sources         []rawSource
	sourceRefs      []rawSourceTag
	byPayloadSource map[rawPayloadSource]rawSourceTag
}

// rawSetRouteFact is the immutable topology projection carried by one
// admitted RawSet operand.  Hot callbacks consume these canonical routes and
// never retain or reopen Topology.
type rawSetRouteFact struct {
	key  heapdomain.Key
	role materialization.Role
	tag  heapdomain.RawRouteTag
}

func buildRawPayloads(topology *Topology, packs *pack.Schema) ([]rawPayload, []rawSource, []rawSourceTag, map[rawPayloadSource]rawSourceTag, bool) {
	if topology == nil || !topology.baseValid() || packs == nil || packs != topology.packs {
		return nil, nil, nil, nil, false
	}
	values := topology.values
	if values == nil || !values.LinkOwner().Matches(packs.LinkOwner()) {
		return nil, nil, nil, nil, false
	}
	result := []rawPayload{{}}
	var sources []rawSource
	var sourceRefs []rawSourceTag
	byPayloadSource := make(map[rawPayloadSource]rawSourceTag)
	sourceTags := make(map[pack.SemanticSource]rawSourceTag)
	visited := 0
	complete := topology.heap.VisitRawPayloadTags(func(tag heapdomain.RawPayloadTag, payload heapdomain.Payload) bool {
		visited = int(tag)
		if int(tag) != len(result) {
			return false
		}
		// Target initial payloads are projected directly by RawAccess. They
		// still occupy their canonical tag position but need no Pack/Value read.
		module, valuesID, offset, programPayload := payload.Source()
		if !programPayload {
			if _, initial := payload.InitialValue(); !initial {
				return false
			}
			result = append(result, rawPayload{kind: rawPayloadInitial})
			return true
		}
		row := rawPayload{}
		mounted, mountedOK := packs.PayloadForMounted(module, valuesID, offset)
		if !mountedOK {
			return false
		}
		switch mounted.Kind() {
		case pack.MountedPayloadFixed:
			fixed, fixedOK := mounted.Fixed()
			if !fixedOK {
				return false
			}
			row.kind = rawPayloadFixed
			coordinate, coordinateOK := values.CoordinateForMountedSemantic(fixed.Module(), fixed.ID())
			if !coordinateOK || !appendRawSource(&sources, &sourceRefs, sourceTags, byPayloadSource, &row, tag, fixed, coordinate) || row.sourceCount != 1 {
				return false
			}
		case pack.MountedPayloadTail:
			payload, payloadOK := mounted.Tail()
			if !payloadOK {
				return false
			}
			row.kind, row.payload = rawPayloadTail, payload
			for sourceIndex := 0; sourceIndex < payload.SourceCount(); sourceIndex++ {
				source, sourceOK := payload.SourceAt(sourceIndex)
				coordinate, coordinateOK := values.CoordinateForMountedSemantic(source.Module(), source.ID())
				if !sourceOK || !coordinateOK || !appendRawSource(&sources, &sourceRefs, sourceTags, byPayloadSource, &row, tag, source, coordinate) {
					return false
				}
			}
		case pack.MountedPayloadNil:
			row.kind = rawPayloadNil
		default:
			return false
		}
		result = append(result, row)
		return true
	})
	// VisitRawPayloadTags permits an intentional early stop.  This constructor
	// has no partial meaning, so every canonical tag must have produced one
	// descriptor; otherwise fail closed instead of returning an empty prefix.
	if !complete || len(result) != visited+1 {
		return nil, nil, nil, nil, false
	}
	return result, sources, sourceRefs, byPayloadSource, true
}

func appendRawSource(all *[]rawSource, refs *[]rawSourceTag, tags map[pack.SemanticSource]rawSourceTag, byPayloadSource map[rawPayloadSource]rawSourceTag, payload *rawPayload, payloadTag heapdomain.RawPayloadTag, source pack.SemanticSource, coordinate valuedomain.Coordinate) bool {
	if all == nil || refs == nil || tags == nil || byPayloadSource == nil || payload == nil || !source.Available() || !coordinate.Valid() || payloadTag == 0 || uint64(len(*all)) == ^uint64(0) || uint64(len(*refs)) >= uint64(^uint32(0)) || payload.sourceCount == ^uint32(0) {
		return false
	}
	key := rawPayloadSource{payload: payloadTag, source: source}
	reverse := payload.kind == rawPayloadTail
	if reverse {
		if _, exists := byPayloadSource[key]; exists {
			return true
		}
	}
	if !reverse && payload.sourceCount != 0 {
		return true
	}
	tag, exists := tags[source]
	if exists {
		row, ok := sourceAt(*all, tag)
		if !ok || row.coordinate != coordinate {
			return false
		}
	} else {
		*all = append(*all, rawSource{coordinate: coordinate})
		tag = rawSourceTag(len(*all))
		tags[source] = tag
	}
	if payload.sourceCount == 0 {
		payload.sourceStart = uint32(len(*refs))
	}
	// The catalog keeps one ordered source-ref vector; each payload retains
	// only a bounded span, avoiding a per-payload slice header/backing array.
	*refs = append(*refs, tag)
	payload.sourceCount++
	if reverse {
		byPayloadSource[key] = tag
	}
	return true
}

func payloadSources(payloads []rawPayload, refs []rawSourceTag, tag heapdomain.RawPayloadTag) ([]rawSourceTag, bool) {
	payload, ok := payloadAt(payloads, tag)
	if !ok {
		return nil, false
	}
	start := uint64(payload.sourceStart)
	count := uint64(payload.sourceCount)
	end := start + count
	if end < start || end > uint64(len(refs)) {
		return nil, false
	}
	return refs[start:end], true
}

func (catalog *rawCatalog) sourceTag(payload heapdomain.RawPayloadTag, source pack.SemanticSource) (rawSourceTag, bool) {
	if catalog == nil || catalog.byPayloadSource == nil || payload == 0 || !source.Available() {
		return 0, false
	}
	tag, ok := catalog.byPayloadSource[rawPayloadSource{payload: payload, source: source}]
	return tag, ok
}

func (catalog *rawCatalog) sourceTags(payload heapdomain.RawPayloadTag) ([]rawSourceTag, bool) {
	if catalog == nil {
		return nil, false
	}
	return payloadSources(catalog.payloads, catalog.sourceRefs, payload)
}

func payloadAt(values []rawPayload, tag heapdomain.RawPayloadTag) (rawPayload, bool) {
	// Compare in the tag's unsigned width before converting to int. A
	// hostile caller can supply a schema-local tag larger than MaxInt; the
	// narrowed comparison would otherwise wrap and index the slice panicking.
	if tag == 0 || uint64(tag) >= uint64(len(values)) {
		return rawPayload{}, false
	}
	value := values[tag]
	return value, value.kind != rawPayloadInvalid
}

func sourceAt(values []rawSource, tag rawSourceTag) (rawSource, bool) {
	if tag == 0 || uint64(tag) > uint64(len(values)) {
		return rawSource{}, false
	}
	return values[tag-1], true
}
