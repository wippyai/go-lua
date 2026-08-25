package index

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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

// RawSourceTag names one semantic source of one payload. It is the catalog's
// own identity for a source row, and it is the identity a raw-access source
// expansion publishes each of its rows under.
type RawSourceTag uint64

type rawPayloadSource struct {
	payload heapdomain.RawPayloadTag
	source  pack.SemanticSource
}

type rawSource struct {
	coordinate valuedomain.Coordinate
}

// rawBootInitial indexes one immutable Target boot slot by the exact route and
// payload tags a hot raw read already holds. Both tags are Heap-issued and
// schema-local, so the index never carries a second key plane.
type rawBootInitial struct {
	route   heapdomain.RawRouteTag
	payload heapdomain.RawPayloadTag
}

// rawCatalog is the one immutable Topology/Link-scoped Pack/Value projection
// shared by both raw operation directions. Heap remains the authority for
// candidate geometry and payload tags; this catalog contains only the Pack
// payload/source rows needed by typed selection callbacks.
type rawCatalog struct {
	payloads        []rawPayload
	sources         []rawSource
	sourceRefs      []RawSourceTag
	byPayloadSource map[rawPayloadSource]RawSourceTag
	bootInitials    map[rawBootInitial]valuedomain.Value
}

// rawSetRouteFact is the immutable topology projection carried by one
// admitted RawSet operand.  Hot callbacks consume these canonical routes and
// never retain or reopen Topology.
type rawSetRouteFact struct {
	key  heapdomain.Key
	role materialization.Role
	tag  heapdomain.RawRouteTag
}

func buildRawPayloads(topology *Topology, packs *pack.Schema) ([]rawPayload, []rawSource, []RawSourceTag, map[rawPayloadSource]RawSourceTag, bool) {
	if topology == nil || !topology.baseValid() || packs == nil || packs != topology.packs {
		return nil, nil, nil, nil, false
	}
	values := topology.values
	if values == nil || !values.LinkOwner().Matches(packs.LinkOwner()) {
		return nil, nil, nil, nil, false
	}
	result := []rawPayload{{}}
	var sources []rawSource
	var sourceRefs []RawSourceTag
	byPayloadSource := make(map[rawPayloadSource]RawSourceTag)
	sourceTags := make(map[pack.SemanticSource]RawSourceTag)
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

// buildRawBootInitials bakes Value's seal-time Target boot-slot projection into
// this Topology's own cold table. Heap's immutable boot rows and Value's
// initial results are both frozen before binding, so every fact is issued once
// by its owner here and indexed by the route and payload tags the hot raw read
// already carries. The hot lane consequently consumes a declared cold receipt
// and never reopens the Value schema to manufacture a peer-domain fact whose
// production no read slot could describe.
//
// A present boot slot with no owner-issued Value is a sealing contradiction,
// not a solve-time miss: Heap admits the row only because Target classified it
// as a stored non-nil initial, which is exactly the domain Value projects.
func buildRawBootInitials(topology *Topology, values *valuedomain.Schema) (map[rawBootInitial]valuedomain.Value, bool) {
	if topology == nil || !topology.baseValid() || values == nil || values != topology.values {
		return nil, false
	}
	heap := topology.heap
	tags := make(map[heapdomain.Payload]heapdomain.RawPayloadTag)
	if !heap.VisitRawPayloadTags(func(tag heapdomain.RawPayloadTag, payload heapdomain.Payload) bool {
		if _, duplicate := tags[payload]; duplicate || tag == 0 {
			return false
		}
		tags[payload] = tag
		return true
	}) {
		return nil, false
	}
	result := make(map[rawBootInitial]valuedomain.Value)
	for index := 0; index < heap.BootEntryCount(); index++ {
		entry, entryOK := heap.BootEntryAt(index)
		if !entryOK {
			return nil, false
		}
		presence, payload, projectionOK := entry.Projection()
		if !projectionOK {
			return nil, false
		}
		// Target keeps Nil and Absent as distinct contract rows; Heap projects
		// both to raw absence, which stores no runtime value at all.
		if presence != heapdomain.RawPresent {
			continue
		}
		key, keyOK := entry.Key()
		root, rootOK := key.BootID()
		initial, initialOK := payload.InitialValue()
		tag, tagOK := tags[payload]
		if !keyOK || !rootOK || !initialOK || !tagOK {
			return nil, false
		}
		value, valueOK := values.TargetInitialID(root, initial)
		if !valueOK {
			return nil, false
		}
		for _, role := range materialization.Roles() {
			// RouteTag is the same producer the hot lane uses to reach this
			// cell, so the roles it admits are exactly the ones to bake.
			route, routeOK := heap.RouteTag(key, role)
			if !routeOK {
				continue
			}
			slot := rawBootInitial{route: route, payload: tag}
			if prior, duplicate := result[slot]; duplicate && !values.Equal(prior, value) {
				return nil, false
			}
			result[slot] = value
		}
	}
	return result, true
}

func appendRawSource(all *[]rawSource, refs *[]RawSourceTag, tags map[pack.SemanticSource]RawSourceTag, byPayloadSource map[rawPayloadSource]RawSourceTag, payload *rawPayload, payloadTag heapdomain.RawPayloadTag, source pack.SemanticSource, coordinate valuedomain.Coordinate) bool {
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
		tag = RawSourceTag(len(*all))
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

func payloadSources(payloads []rawPayload, refs []RawSourceTag, tag heapdomain.RawPayloadTag) ([]RawSourceTag, bool) {
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

func (catalog *rawCatalog) sourceTag(payload heapdomain.RawPayloadTag, source pack.SemanticSource) (RawSourceTag, bool) {
	if catalog == nil || catalog.byPayloadSource == nil || payload == 0 || !source.Available() {
		return 0, false
	}
	tag, ok := catalog.byPayloadSource[rawPayloadSource{payload: payload, source: source}]
	return tag, ok
}

func (catalog *rawCatalog) sourceTags(payload heapdomain.RawPayloadTag) ([]RawSourceTag, bool) {
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

func sourceAt(values []rawSource, tag RawSourceTag) (rawSource, bool) {
	if tag == 0 || uint64(tag) > uint64(len(values)) {
		return rawSource{}, false
	}
	return values[tag-1], true
}
