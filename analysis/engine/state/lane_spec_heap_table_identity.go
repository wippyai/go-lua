package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

const LaneHeapTableIdentity LaneID = "heap-table-identity"

var heapTableIdentityLaneSpec = laneSpec{
	id:           LaneHeapTableIdentity,
	keySpaceMode: laneKeySpaceOwned,
	boundary:     boundaryLaneOps{expand: expandHeapTableIdentityBoundary, project: projectHeapBoundary, rebase: rebaseHeapBoundary, apply: applyHeapBoundary, equal: equalHeapBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.heapTableIdentity.rekey(from, to)
		if !ok {
			return s, false
		}
		s.heapTableIdentity = lane
		return s, true
	},
	fingerprint: fingerprintHeapTableIdentity,
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := heapidentity.MapDomain(reg)
		return stateLane(domain,
			func(s State) map[identity.ID]heapidentity.TableObject {
				return s.heapTableIdentity.asMap(domain)
			},
			func(out *State, objects map[identity.ID]heapidentity.TableObject) {
				out.heapTableIdentity = heapTableIdentityLaneFromMap(domain, objects)
			},
		)
	},
}

func expandHeapTableIdentityBoundary(expansion *boundaryClosureExpansion, source State) {
	if source.heapTableIdentity.top {
		return
	}
	ids := expansion.closure.identities
	if expansion.closure.allIdentities {
		ids = make(map[identity.ID]struct{}, len(source.heapTableIdentity.values))
		for id := range source.heapTableIdentity.values {
			ids[id] = struct{}{}
			expansion.addIdentity(id)
		}
	}
	for id := range ids {
		object, ok := source.heapTableIdentity.values[id]
		if !ok {
			continue
		}
		expansion.addValue(object.Root())
		for path, value := range object.StaticMembers() {
			expansion.addHeapSuffix(id, path)
			expansion.addValue(value)
		}
		for factKey, fact := range object.DynamicIndexFacts() {
			expansion.addHeapSuffix(id, factKey.Table)
			expansion.addValue(fact.KeyValue)
			expansion.addValue(fact.Value)
		}
	}
}
