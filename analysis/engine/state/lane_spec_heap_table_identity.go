package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

const LaneHeapTableIdentity LaneID = "heap-table-identity"

var heapTableIdentityLaneSpec = laneSpec{
	id: LaneHeapTableIdentity,
	build: func(reg *axis.Registry) laneOps {
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
