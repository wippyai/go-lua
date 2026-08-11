package index

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

type rawPayloadKind uint8

const (
	rawPayloadInvalid rawPayloadKind = iota
	rawPayloadFixed
	rawPayloadTail
	rawPayloadNil
	rawPayloadInitial
)

// rawPayload is a cold descriptor for one existing Heap RawPayloadTag. Values
// is Pack's Program-source selector and payload is its owner-fenced marginal.
// Neither is recurrent fact state.
type rawPayload struct {
	kind    rawPayloadKind
	values  pack.Values
	payload pack.Payload
	fixed   linkboundary.Value
	// source is the exact Program Values source retained by Heap's Payload.
	// It is declaration-time provenance only; IndexAccessGeometry remains the
	// authority for admitting an indexed write.
	source  rawPayloadSource
	sources []rawSourceTag
	byValue map[linkboundary.Value]rawSourceTag
}

type rawPayloadSource struct {
	shard  linkproject.Shard
	values keyspace.Term
	offset int
}

type rawSourceTag uint64

type rawSource struct {
	value      linkboundary.Value
	coordinate valuedomain.Coordinate
}

func buildRawPayloads(topology *Topology, packs *pack.Schema) ([]rawPayload, []rawSource, bool) {
	if topology == nil || !topology.valid() || packs == nil {
		return nil, nil, false
	}
	values := topology.values
	linked := values.Link()
	if linked == nil || linked.Boundary() == nil {
		return nil, nil, false
	}
	boundaryValues := linked.Boundary().Values()
	result := []rawPayload{{}}
	requests := make([]pack.PayloadRequest, 0)
	tailRows := make([]int, 0)
	var sources []rawSource
	visited := 0
	complete := topology.heap.VisitRawPayloadTags(func(tag heapdomain.RawPayloadTag, payload heapdomain.Payload) bool {
		visited = int(tag)
		if int(tag) != len(result) {
			return false
		}
		// Target initial payloads are projected directly by RawAccess. They
		// still occupy their canonical tag position but need no Pack/Value read.
		shard, valuesTerm, offset, programPayload := payload.Source()
		if !programPayload {
			if _, initial := payload.InitialValue(); !initial {
				return false
			}
			result = append(result, rawPayload{kind: rawPayloadInitial})
			return true
		}
		p, programOK := linked.Project().Mounts().Program(shard)
		if !programOK || p == nil {
			return false
		}
		row := rawPayload{source: rawPayloadSource{shard: shard, values: valuesTerm, offset: offset}}
		position, ok := p.Flow().Authored().Values().Position(valuesTerm, offset)
		if !ok {
			return false
		}
		switch {
		case position.Fixed != 0:
			fixed, fixedOK := boundaryValues.Of(shard, position.Fixed)
			if !fixedOK {
				return false
			}
			row.kind, row.fixed = rawPayloadFixed, fixed
		case position.Tail != 0:
			// The Pack relation belongs to the complete executable Values
			// occurrence named by Heap's payload.  position.Tail is only the
			// symbolic Call/Vararg producer for its open suffix and deliberately
			// has no competing Values root.
			packValues, _, valuesOK := packs.Values(shard, valuesTerm)
			if !valuesOK {
				return false
			}
			row.kind, row.values = rawPayloadTail, packValues
			requests = append(requests, pack.PayloadRequest{Values: packValues, Index: offset})
			tailRows = append(tailRows, len(result))
		case position.NilFill:
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
		return nil, nil, false
	}
	selections, selectionsOK := packs.Payloads(requests)
	if !selectionsOK || len(selections) != len(tailRows) {
		return nil, nil, false
	}
	for selectionIndex, rowIndex := range tailRows {
		if rowIndex <= 0 || rowIndex >= len(result) {
			return nil, nil, false
		}
		result[rowIndex].payload = selections[selectionIndex]
	}
	sourceTags := make(map[linkboundary.Value]rawSourceTag)
	for rowIndex := 1; rowIndex < len(result); rowIndex++ {
		row := &result[rowIndex]
		switch row.kind {
		case rawPayloadFixed:
			coordinate, found := values.CoordinateFor(row.fixed)
			if !found || !appendRawSource(&sources, sourceTags, row, row.fixed, coordinate) {
				return nil, nil, false
			}
		case rawPayloadTail:
			for sourceIndex := 0; sourceIndex < row.payload.SourceCount(); sourceIndex++ {
				source, sourceOK := row.payload.SourceAt(sourceIndex)
				coordinate, coordinateOK := values.CoordinateFor(source)
				if !sourceOK || !coordinateOK || !appendRawSource(&sources, sourceTags, row, source, coordinate) {
					return nil, nil, false
				}
			}
		}
	}
	return result, sources, true
}

func appendRawSource(all *[]rawSource, tags map[linkboundary.Value]rawSourceTag, payload *rawPayload, value linkboundary.Value, coordinate valuedomain.Coordinate) bool {
	if all == nil || tags == nil || payload == nil || value == (linkboundary.Value{}) || !coordinate.Valid() || uint64(len(*all)) == ^uint64(0) {
		return false
	}
	if payload.byValue == nil {
		payload.byValue = make(map[linkboundary.Value]rawSourceTag)
	}
	if _, exists := payload.byValue[value]; exists {
		return true
	}
	tag, exists := tags[value]
	if exists {
		source, ok := sourceAt(*all, tag)
		if !ok || source.value != value || source.coordinate != coordinate {
			return false
		}
	} else {
		*all = append(*all, rawSource{value: value, coordinate: coordinate})
		tag = rawSourceTag(len(*all))
		tags[value] = tag
	}
	payload.sources = append(payload.sources, tag)
	payload.byValue[value] = tag
	return true
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
