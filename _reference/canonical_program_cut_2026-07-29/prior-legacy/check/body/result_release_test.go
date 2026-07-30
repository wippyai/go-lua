package body

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestReleaseTransientDropsSealedFlowAndWorkingSet(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	result := &Result{
		registry: reg,
		flow:     transfer.Result{point: state.State{}},
		published: PublishedFacts{
			nodeOutputs:         map[cfg.Point]state.State{point: {}},
			pointReachable:      map[cfg.Point]bool{point: true},
			nodeOutputReachable: map[cfg.Point]bool{point: true},
			edgeNormal:          map[observationEdge]bool{{from: point, to: point}: true},
		},
		observationPlan: ObservationPlan{boundarySet: map[cfg.Point]struct{}{point: {}}},
	}

	result.ReleaseTransient()
	if result.flow != nil {
		t.Fatal("ReleaseTransient retained solved flow")
	}
	if result.published.nodeOutputs != nil || result.published.pointReachable != nil || result.published.nodeOutputReachable != nil || result.published.edgeNormal != nil {
		t.Fatal("ReleaseTransient retained sealed state maps")
	}
	if result.observationPlan.boundarySet != nil {
		t.Fatal("ReleaseTransient retained observation plan")
	}
}
