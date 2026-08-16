package causal

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestInstallBoundRoutePathsRejectsPresentBoundaryWithoutPlanReceipt(t *testing.T) {
	id := keyspace.ContentID{1}
	path := keyspace.ContentID{2}
	route := successorRef{routeDigest: id, routeIndexOrdinal: 0, planOrdinal: 0, planOrdinalSet: true}
	r := &Result{
		boundRouteReceipts: []boundRouteReceipt{{fromPath: path, toPath: path}},
		index:              successorIndex{refs: []successorRef{route}},
		routeIndex:         []routeLookup{{digest: id, ref: route}},
		boundaries: boundaryStore{rows: []boundaryRow{{
			CallBoundary: CallBoundary{Throw: keyspace.MakeTerm(keyspace.FamilyOutcome, 1)},
			refs: [BoundaryCancel + 1]successorRef{
				BoundaryThrow: {routeDigest: id, routeIndexOrdinal: 0, planOrdinal: 0, planOrdinalSet: true},
			},
		}}},
	}
	if r.installBoundRoutePaths() == nil {
		t.Fatal("present boundary arm without plan receipt was accepted")
	}
}
