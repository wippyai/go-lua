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
)

type stabilityCacheResult struct {
	graph        *cfg.CFG
	callSites    map[cfg.Point]factflow.CallSite
	callOutcomes map[cfg.Point]callpayload.CallOutcome
	rootAssigns  map[cfg.Point]factflow.RootAssignment
	outcomeReads int
	rootReads    int
}

func (r *stabilityCacheResult) RootAssignment(point cfg.Point) (factflow.RootAssignment, bool) {
	r.rootReads++
	assignment, ok := r.rootAssigns[point]
	return assignment, ok
}

func (r *stabilityCacheResult) Registry() *axis.Registry         { return nil }
func (r *stabilityCacheResult) Graph() cfg.Graph                 { return r.graph }
func (r *stabilityCacheResult) ExitState() (state.State, bool)   { return state.State{}, false }
func (r *stabilityCacheResult) ReturnPoints() []cfg.Point        { return nil }
func (r *stabilityCacheResult) KeySpace() *keyspace.KeySpace     { return nil }
func (r *stabilityCacheResult) ParameterValueSlots() []key.Value { return nil }
func (r *stabilityCacheResult) CallSiteView(point cfg.Point) (factflow.CallSiteView, bool) {
	site, ok := r.callSites[point]
	return site.View(), ok
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

func TestStableLocalPathSourceUsesExactSharedAssignmentIndex(t *testing.T) {
	graph := cfg.New()
	declaration := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), declaration, true)
	previous := declaration
	for range 64 {
		point := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(previous, point, true)
		previous = point
	}
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(previous, use, true)
	graph.AddEdge(use, graph.Exit(), true)

	local := pathdom.NewPath(7, "local")
	source := factflow.ValueSource{Kind: factflow.ValueSourceNil}
	result := &stabilityCacheResult{
		graph: graph,
		rootAssigns: map[cfg.Point]factflow.RootAssignment{
			declaration: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, local.Symbol, local, source),
		},
	}
	cache := newParamObligationProjectorCache(graph)
	projector := newParamObligationProjector(nil, result, nil, graph, cache)
	projector.point = use
	indexedReads := result.rootReads

	for range 4 {
		got, ok := projector.stableLocalPathSource(local)
		if !ok || got.Kind != factflow.ValueSourceNil {
			t.Fatalf("stable source = (%v, %v), want nil source", got, ok)
		}
	}
	if result.rootReads != indexedReads {
		t.Fatalf("root assignment reads after indexing = %d, want %d", result.rootReads, indexedReads)
	}
}

func TestStableLocalPathSourceIndexesOrdinaryWriteTargetPathSymbol(t *testing.T) {
	graph := cfg.New()
	declaration := graph.AddNode(cfg.NodeAssign)
	write := graph.AddNode(cfg.NodeAssign)
	use := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), declaration, true)
	graph.AddEdge(declaration, write, true)
	graph.AddEdge(write, use, true)
	graph.AddEdge(use, graph.Exit(), true)

	local := pathdom.NewPath(7, "local")
	result := &stabilityCacheResult{
		graph: graph,
		rootAssigns: map[cfg.Point]factflow.RootAssignment{
			declaration: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, local.Symbol, local, factflow.ValueSource{Kind: factflow.ValueSourceNil}),
			write:       factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, 8, local, factflow.ValueSource{Kind: factflow.ValueSourceNil}),
		},
	}
	projector := newParamObligationProjector(nil, result, nil, graph, newParamObligationProjectorCache(graph))
	projector.point = use
	if source, ok := projector.stableLocalPathSource(local); ok {
		t.Fatalf("stable source = %v, want invalidated", source)
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
