package index

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// This file is the owner's raw-access judgment surface: the mathematics of the
// indexed read and write, stated in owner types alone.
//
// A judgment here observes and answers. It selects no route, stages nothing,
// and reads no engine state, because the relational plan for raw access makes
// each dependent read an expansion that publishes a route relation and then an
// ordinary equijoin onto it. What the owner owes that plan is exactly the
// enumeration each expansion is over, and that is what these methods are.
//
// The hot rule calls the same methods. There is one statement of each of these
// enumerations in the analyzer, and both the standing plan and the protocol it
// replaces reach it here.

// RawPayload is one payload descriptor the catalog holds, addressed by the
// Heap-issued tag its route names it under.
type RawPayload struct {
	tag heapdomain.RawPayloadTag
	row rawPayload
}

// Available reports whether the descriptor addresses a catalog row.
func (payload RawPayload) Available() bool {
	return payload.tag != 0 && payload.row.kind != rawPayloadInvalid
}

// Tag returns the Heap-issued identity of this payload.
func (payload RawPayload) Tag() heapdomain.RawPayloadTag { return payload.tag }

// IsTail reports whether this payload is the open tail of its pack, which is
// the only payload kind a pack route is published for.
func (payload RawPayload) IsTail() bool { return payload.row.kind == rawPayloadTail }

// IsFixed reports whether this payload is a fixed slot of its pack, which is
// the payload kind a write answers from a semantic source rather than a root.
func (payload RawPayload) IsFixed() bool { return payload.row.kind == rawPayloadFixed }

// Root returns the pack root this payload projects, when it has one.
func (payload RawPayload) Root() (pack.Root, bool) {
	if !payload.Available() {
		return pack.Root{}, false
	}
	return payload.row.payload.Root()
}

// RawPayloadAt answers the catalog descriptor of one payload tag.
func (topology *Topology) RawPayloadAt(tag heapdomain.RawPayloadTag) (RawPayload, bool) {
	if topology == nil || !topology.valid() {
		return RawPayload{}, false
	}
	row, ok := payloadAt(topology.catalog.payloads, tag)
	if !ok {
		return RawPayload{}, false
	}
	return RawPayload{tag: tag, row: row}, true
}

// CoordinateName answers the portable identity the sealed value schema issued
// for one of its coordinates. A raw-access route whose destination is a value
// coordinate publishes its row under this name, so the row is addressed by the
// identity the coordinate's own owner assigned and never by one the route
// derived.
func (topology *Topology) CoordinateName(coordinate valuedomain.Coordinate) (identity.ContentID, bool) {
	if topology == nil || !topology.valid() {
		return identity.ContentID{}, false
	}
	return topology.values.CoordinateContentID(coordinate)
}

// PackRootName answers the portable identity the sealed pack schema issued for
// one of its roots, which is the name a pack route publishes its row under.
func (topology *Topology) PackRootName(root pack.Root) (identity.ContentID, bool) {
	if topology == nil || !topology.valid() || topology.packs == nil {
		return identity.ContentID{}, false
	}
	return topology.packs.RootID(root)
}

// RawWritePayload answers the payload descriptor one write candidate
// addresses. The tag is reissued from the candidate's own access geometry, so
// the descriptor is the one Heap named and never one this layer chose.
func (topology *Topology) RawWritePayload(access Index) (RawPayload, bool) {
	if topology == nil || !topology.valid() || !access.valid() || access.topology != topology {
		return RawPayload{}, false
	}
	tag, ok := topology.heap.RawPayloadTagForIndexAccess(access.indexAccess)
	if !ok {
		return RawPayload{}, false
	}
	return topology.RawPayloadAt(tag)
}

// RawSourceCoordinate answers the value coordinate one semantic source names.
func (topology *Topology) RawSourceCoordinate(tag RawSourceTag) (valuedomain.Coordinate, bool) {
	if topology == nil || !topology.valid() {
		return valuedomain.Coordinate{}, false
	}
	source, ok := sourceAt(topology.catalog.sources, tag)
	if !ok {
		return valuedomain.Coordinate{}, false
	}
	return source.coordinate, true
}

// RawBootInitial answers the sealed boot value one route and payload address,
// when the target declares one.
func (topology *Topology) RawBootInitial(route heapdomain.RawRouteTag, payload heapdomain.RawPayloadTag) (valuedomain.Value, bool) {
	if topology == nil || !topology.valid() {
		return valuedomain.Value{}, false
	}
	value, ok := topology.catalog.bootInitials[rawBootInitial{route: route, payload: payload}]
	return value, ok
}

// VisitPayloadSources enumerates every semantic source one payload declares,
// in the catalog's own order. It is the enumeration a raw-access source
// expansion is over: each visited source is one published row, named by the
// coordinate its own tag addresses.
func (topology *Topology) VisitPayloadSources(payload heapdomain.RawPayloadTag, visit func(RawSourceTag, valuedomain.Coordinate) bool) bool {
	if topology == nil || !topology.valid() || visit == nil {
		return false
	}
	tags, ok := topology.catalog.sourceTags(payload)
	if !ok {
		return false
	}
	for _, tag := range tags {
		coordinate, coordinateOK := topology.RawSourceCoordinate(tag)
		if !coordinateOK || !visit(tag, coordinate) {
			return false
		}
	}
	return true
}

// VisitRoutePayloads enumerates every payload one selected heap route fact
// carries under a key selector. It is the enumeration a raw-access pack
// expansion is over.
//
// A target boot payload has no program descriptor and is not a catalog row;
// the enumeration passes over it rather than refusing, because the boot value
// is answered by RawBootInitial and never by a payload descriptor.
func (topology *Topology) VisitRoutePayloads(route heapdomain.RawRouteTag, fact heapdomain.Value, selector heapdomain.KeySelector, visit func(RawPayload) bool) bool {
	if topology == nil || !topology.valid() || visit == nil {
		return false
	}
	return topology.heap.VisitRawAccessRoute(route, fact, selector, func(raw heapdomain.RawAccess) bool {
		if raw.IsTop() {
			return true
		}
		cell, ok := raw.Cell()
		if !ok {
			return false
		}
		for index := 0; index < cell.PresentCount(); index++ {
			present, presentOK := cell.PresentAt(index)
			if !presentOK {
				return false
			}
			tag, tagged := raw.PayloadTag(present)
			if !tagged {
				if _, _, initial := raw.InitialPayload(present); initial {
					continue
				}
				return false
			}
			payload, payloadOK := topology.RawPayloadAt(tag)
			if !payloadOK || !visit(payload) {
				return false
			}
		}
		return true
	})
}
