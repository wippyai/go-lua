package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func testInitialStatePlan(
	t *testing.T,
	owner lexicalidentity.StableLexicalBodyID,
	graph cfg.Graph,
	seeds ...state.InitialStateSeed,
) state.InitialStatePlan {
	t.Helper()
	points := cfg.RPOReadOnly(graph)
	ordered := make([]state.InitialCoordinate, len(points))
	for index, point := range points {
		ordered[index] = state.InitialCoordinate(point)
	}
	plan, err := state.NewInitialStatePlan(owner, graph.ID(), graph.Size(), ordered, seeds)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func testAcyclicCallTopology(
	t *testing.T,
	bodies ...lexicalidentity.StableLexicalBodyID,
) operationplan.CallTopology {
	t.Helper()
	boundaries := make([]operationplan.CallTopologyBoundaryInput, len(bodies))
	for index, body := range bodies {
		boundaries[index] = operationplan.CallTopologyBoundaryInput{Body: body}
	}
	topology, err := operationplan.SealCallTopology(bodies, nil, nil, boundaries)
	if err != nil {
		t.Fatal(err)
	}
	return topology
}
