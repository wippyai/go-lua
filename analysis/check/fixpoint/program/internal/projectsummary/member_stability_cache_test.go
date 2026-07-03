package projectsummary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

type stabilityCacheResult struct {
	graph        *cfg.CFG
	callSites    map[cfg.Point]factflow.CallSite
	callOutcomes map[cfg.Point]callpayload.CallOutcome
	outcomeReads int
}

func (r *stabilityCacheResult) Registry() *axis.Registry         { return nil }
func (r *stabilityCacheResult) Graph() cfg.Graph                 { return r.graph }
func (r *stabilityCacheResult) ExitState() (state.State, bool)   { return state.State{}, false }
func (r *stabilityCacheResult) ReturnPoints() []cfg.Point        { return nil }
func (r *stabilityCacheResult) KeySpace() *keyspace.KeySpace     { return nil }
func (r *stabilityCacheResult) ParameterValueSlots() []key.Value { return nil }
func (r *stabilityCacheResult) Call(cfg.Point) (semantics.CallFact, bool) {
	return semantics.CallFact{}, true
}

func (r *stabilityCacheResult) CallSite(point cfg.Point) (factflow.CallSite, bool) {
	site, ok := r.callSites[point]
	return site, ok
}

func (r *stabilityCacheResult) CallOutcomeAt(point cfg.Point) (callpayload.CallOutcome, bool) {
	r.outcomeReads++
	outcome, ok := r.callOutcomes[point]
	return outcome, ok
}

func TestMemberCallReceiverStableCachesIdenticalQueries(t *testing.T) {
	graph, prior, current := stabilityCacheGraph()
	receiver := pathdom.NewPath(1, "client")
	member := segment.Segment{Kind: segment.SegmentField, Name: "invoke"}
	result := newStabilityCacheResult(graph, prior)
	projector := paramObligationProjector{
		result:    result,
		reach:     cfg.NewReachability(graph),
		stability: make(map[memberReceiverStabilityKey]bool),
		point:     current,
	}

	if !projector.memberCallReceiverStable(receiver, member) {
		t.Fatalf("first stability query = false, want true")
	}
	if !projector.memberCallReceiverStable(receiver, member) {
		t.Fatalf("second stability query = false, want cached true")
	}
	if result.outcomeReads != 1 {
		t.Fatalf("CallOutcomeAt reads = %d, want 1 cached prior-outcome read", result.outcomeReads)
	}
}

func TestMemberCallReceiverStableCacheCanBeSharedAcrossProjectors(t *testing.T) {
	graph, prior, current := stabilityCacheGraph()
	receiver := pathdom.NewPath(1, "client")
	member := segment.Segment{Kind: segment.SegmentField, Name: "invoke"}
	result := newStabilityCacheResult(graph, prior)
	cache := newParamObligationProjectorCache(graph)
	first := newParamObligationProjector(nil, result, nil, graph, cache)
	first.point = current
	second := newParamObligationProjector(nil, result, nil, graph, cache)
	second.point = current

	if !first.memberCallReceiverStable(receiver, member) {
		t.Fatalf("first stability query = false, want true")
	}
	if !second.memberCallReceiverStable(receiver, member) {
		t.Fatalf("second stability query = false, want cached true")
	}
	if result.outcomeReads != 1 {
		t.Fatalf("CallOutcomeAt reads = %d, want 1 shared-cache prior-outcome read", result.outcomeReads)
	}
}

func stabilityCacheGraph() (*cfg.CFG, cfg.Point, cfg.Point) {
	graph := cfg.New()
	prior := graph.AddNode(cfg.NodeCall)
	current := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), prior, true)
	graph.AddEdge(prior, current, true)
	graph.AddEdge(current, graph.Exit(), true)
	return graph, prior, current
}

func newStabilityCacheResult(graph *cfg.CFG, prior cfg.Point) *stabilityCacheResult {
	return &stabilityCacheResult{
		graph: graph,
		callSites: map[cfg.Point]factflow.CallSite{
			prior: factflow.NewCallSite(factflow.CallSiteConfig{}),
		},
		callOutcomes: map[cfg.Point]callpayload.CallOutcome{
			prior: {},
		},
	}
}
