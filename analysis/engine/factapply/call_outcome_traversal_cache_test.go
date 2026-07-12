package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCallOutcomeTraversalCacheBuildsExactCallSitesOnce(t *testing.T) {
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeCall)
	prev := first
	graph.AddEdge(graph.Entry(), first, false)
	for range 64 {
		point := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(prev, point, false)
		prev = point
	}
	second := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(prev, second, false)
	graph.AddEdge(second, graph.Exit(), false)

	facts := factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{
		first:  factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		second: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
	}})
	stats := &callOutcomeTraversalStats{}
	cache := &callOutcomeTraversalCache{stats: stats}

	firstRead := cache.exactCallSites(graph, facts)
	secondRead := cache.exactCallSites(graph, facts)

	if len(firstRead) != 2 || len(secondRead) != 2 {
		t.Fatalf("exact call sites = %d, %d; want 2, 2", len(firstRead), len(secondRead))
	}
	if firstRead[0].point != first || firstRead[1].point != second {
		t.Fatalf("exact call-site order = %v, %v; want %v, %v", firstRead[0].point, firstRead[1].point, first, second)
	}
	if stats.callSiteBuilds != 1 {
		t.Fatalf("call-site cache builds = %d, want 1", stats.callSiteBuilds)
	}
	if stats.callSitePointProbes != len(graph.RPO()) {
		t.Fatalf("call-site point probes = %d, want one RPO scan of %d", stats.callSitePointProbes, len(graph.RPO()))
	}
}

func TestCallOutcomePresencePublishFetchesAssignmentSourceCallOnly(t *testing.T) {
	graph := cfg.New()
	callA := graph.AddNode(cfg.NodeCall)
	assignA0 := graph.AddNode(cfg.NodeAssign)
	assignA1 := graph.AddNode(cfg.NodeAssign)
	callB := graph.AddNode(cfg.NodeCall)
	assignB0 := graph.AddNode(cfg.NodeAssign)
	assignB1 := graph.AddNode(cfg.NodeAssign)
	points := []cfg.Point{callA, assignA0, assignA1, callB, assignB0, assignB1, graph.Exit()}
	prev := graph.Entry()
	for _, point := range points {
		graph.AddEdge(prev, point, false)
		prev = point
	}

	a0, a1 := symbol.ID(801), symbol.ID(802)
	b0, b1 := symbol.ID(803), symbol.ID(804)
	a0Path, a1Path := pathdom.NewPath(a0, "a0"), pathdom.NewPath(a1, "a1")
	b0Path, b1Path := pathdom.NewPath(b0, "b0"), pathdom.NewPath(b1, "b1")
	site := func(first, second symbol.ID, firstPath, secondPath pathdom.Path) factflow.CallSite {
		return factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextAssignmentSource,
			ResultTargets: []factflow.CallResultTarget{
				factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, first, firstPath),
				factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, second, secondPath),
			},
		})
	}
	assignment := func(target symbol.ID, targetPath pathdom.Path, call cfg.Point, index int) factflow.RootAssignment {
		return factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, factflow.ValueSource{
			Kind: factflow.ValueSourceCall, TargetIndex: index, ResultIndex: index, CallPoint: call, HasCallPoint: true,
		})
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			callA: site(a0, a1, a0Path, a1Path),
			callB: site(b0, b1, b0Path, b1Path),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignA0: assignment(a0, a0Path, callA, 0),
			assignA1: assignment(a1, a1Path, callA, 1),
			assignB0: assignment(b0, b0Path, callB, 0),
			assignB1: assignment(b1, b1Path, callB, 1),
		},
	})
	var providerPoints []cfg.Point
	provider := func(ctx transfer.NodeContext, _ factflow.CallSiteView, _ state.State, _ func(cfg.Point) state.State) callpayload.CallOutcome {
		providerPoints = append(providerPoints, ctx.Point)
		return callpayload.CallOutcome{}
	}
	stats := &callOutcomeTraversalStats{}
	cache := &callOutcomeTraversalCache{stats: stats}
	resolver := visibility.NewResolver(visibility.NewBuilder().Build())

	applyCallOutcomePresenceRelationPublishes(transfer.NodeContext{
		Graph: graph, Registry: standard.Registry(), Point: assignB1, Node: graph.Node(assignB1),
	}, facts, cache, provider, resolver, nil, state.State{})

	if len(providerPoints) != 1 || providerPoints[0] != callB {
		t.Fatalf("provider points = %v, want only assignment source call %v", providerPoints, callB)
	}
	if stats.presenceDirectLookups != 1 {
		t.Fatalf("presence direct lookups = %d, want 1", stats.presenceDirectLookups)
	}
	if stats.callSitePointProbes != 0 {
		t.Fatalf("presence publication scanned %d CFG points, want 0", stats.callSitePointProbes)
	}
}

func TestCallOutcomeEdgeTraversalVisitsOnlyExactCallEntries(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	statementCall := graph.AddNode(cfg.NodeCall)
	prev := statementCall
	graph.AddEdge(graph.Entry(), statementCall, false)
	for range 64 {
		point := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(prev, point, false)
		prev = point
	}
	conditionCall := graph.AddNode(cfg.NodeCall)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(prev, conditionCall, false)
	graph.AddEdge(conditionCall, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)
	facts := factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{
		statementCall: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		conditionCall: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextCondition}),
	}})
	providerCalls := 0
	provider := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		providerCalls++
		return callpayload.CallOutcome{}
	}
	stats := &callOutcomeTraversalStats{}
	executor := &ResolvedCallOutcomeEdgeExecutor{cache: callOutcomeTraversalCache{stats: stats}}
	ctx := transfer.EdgeContext{
		Graph: graph, Registry: reg, Edge: cfg.Edge{From: branch, To: thenPoint, Cond: true}, HasCond: true,
	}

	applyCallOutcomeEdgeFacts(ctx, facts, executor, provider, nil, nil, nil, state.State{})
	applyCallOutcomeEdgeFacts(ctx, facts, executor, provider, nil, nil, nil, state.State{})

	if providerCalls != 2 {
		t.Fatalf("provider calls = %d, want one condition call per edge transfer", providerCalls)
	}
	if stats.callSiteBuilds != 1 || stats.callSitePointProbes != len(graph.RPO()) {
		t.Fatalf("cache builds/probes = %d/%d, want 1/%d", stats.callSiteBuilds, stats.callSitePointProbes, len(graph.RPO()))
	}
	if stats.edgeCallEntriesVisited != 4 {
		t.Fatalf("edge call entries visited = %d, want two exact entries per transfer", stats.edgeCallEntriesVisited)
	}
}

func BenchmarkFactsEdgeTransferSparseCallSites(b *testing.B) {
	reg := standard.Registry()
	graph := cfg.New()
	prev := graph.Entry()
	for range 1024 {
		point := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(prev, point, false)
		prev = point
	}
	call := graph.AddNode(cfg.NodeCall)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(prev, call, false)
	graph.AddEdge(call, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)
	facts := factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{
		call: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextCondition}),
	}})
	transferFn := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts: facts,
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{}
		},
	})
	ctx := transfer.EdgeContext{
		Graph: graph, Registry: reg, Edge: cfg.Edge{From: branch, To: thenPoint, Cond: true}, HasCond: true,
	}
	b.ResetTimer()
	for range b.N {
		transferFn(ctx, state.State{})
	}
	b.ReportMetric(float64(len(graph.RPO())), "graph-points")
	b.ReportMetric(float64(facts.CallSiteCount()), "call-sites")
}
