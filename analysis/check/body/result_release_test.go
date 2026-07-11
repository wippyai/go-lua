package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestReleaseTransientDropsSealedFlowAndWorkingSet(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	result := &Result{
		registry:     reg,
		flow:         transfer.Result{point: state.State{}},
		boundaryXfer: func(transfer.NodeContext, state.State) state.State { return state.State{} },
		edgeXfer:     func(transfer.EdgeContext, state.State) state.State { return state.State{} },
		published: PublishedFacts{
			nodeOutputs:         map[cfg.Point]state.State{point: {}},
			pointReachable:      map[cfg.Point]bool{point: true},
			nodeOutputReachable: map[cfg.Point]bool{point: true},
			edgeNormal:          map[observationEdge]bool{{from: point, to: point}: true},
		},
		capturedNodeOutputs: map[cfg.Point]state.State{point: {}},
		observationPlan:     ObservationPlan{boundarySet: map[cfg.Point]struct{}{point: {}}},
	}

	result.ReleaseTransient()
	if result.flow != nil {
		t.Fatal("ReleaseTransient retained solved flow")
	}
	if result.boundaryXfer != nil || result.edgeXfer != nil {
		t.Fatal("ReleaseTransient retained transfer closures")
	}
	if result.published.nodeOutputs != nil || result.published.pointReachable != nil || result.published.nodeOutputReachable != nil || result.published.edgeNormal != nil || result.capturedNodeOutputs != nil {
		t.Fatal("ReleaseTransient retained sealed state maps")
	}
	if result.observationPlan.boundarySet != nil {
		t.Fatal("ReleaseTransient retained observation plan")
	}
}
