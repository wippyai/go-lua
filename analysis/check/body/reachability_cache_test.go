package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func slowResultStateReachable(result *Result, in state.State) bool {
	domain, err := state.TryDomainWithOptionalLanes(result.registry, result.stateLanes)
	if err != nil {
		domain = state.Domain(result.registry)
	}
	return !domain.Equal(state.NormalizeForDomain(domain, in), domain.Bottom())
}

func TestPublishedPointReachabilityMatchesSlowDomainBottomAcrossLaneSelections(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.FromType(reg, typ.String)
	valueState := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(1)), value)
	tests := []struct {
		name  string
		lanes []state.LaneID
		flow  transfer.Result
	}{
		{"default", nil, transfer.Result{1: state.Domain(reg).Bottom(), 2: state.State{}, 3: valueState}},
		{"empty", []state.LaneID{}, transfer.Result{1: state.State{}, 2: valueState}},
		{"values", []state.LaneID{state.LaneValues}, transfer.Result{1: state.State{}, 2: valueState}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := cfg.New()
			result := &Result{
				registry:     reg,
				cfg:          &cfgbuild.Result{Graph: graph},
				flow:         test.flow,
				stateLanes:   test.lanes,
				boundaryXfer: func(_ transfer.NodeContext, in state.State) state.State { return in },
			}
			result.sealObservations()
			if result.published.pointReachable == nil {
				t.Fatal("seal did not publish point reachability")
			}
			for point, in := range test.flow {
				want := slowResultStateReachable(result, in)
				if cached, ok := result.publishedPointReachable(point); !ok || cached != want {
					t.Fatalf("point %d cache=%v/%v slow=%v", point, cached, ok, want)
				}
				if got := result.PointReachable(point); got != want {
					t.Fatalf("PointReachable(%d)=%v slow=%v", point, got, want)
				}
				fallback := *result
				fallback.published = PublishedFacts{}
				if got := fallback.PointReachable(point); got != want {
					t.Fatalf("fallback PointReachable(%d)=%v slow=%v", point, got, want)
				}
			}
		})
	}
}

func TestPublishedEdgeReachabilityMatchesSlowBranchAndNoNormalReturn(t *testing.T) {
	reg := standard.Registry()
	reachable := state.State{}

	t.Run("branch", func(t *testing.T) {
		graph := cfg.New()
		branch := graph.AddNode(cfg.NodeBranch)
		left := graph.AddNode(cfg.NodeAssign)
		right := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(graph.Entry(), branch, false)
		graph.AddEdge(branch, left, false)
		graph.AddEdge(branch, right, true)
		graph.AddEdge(left, graph.Exit(), false)
		graph.AddEdge(right, graph.Exit(), false)
		domain := state.Domain(reg)
		result := &Result{
			registry:     reg,
			cfg:          &cfgbuild.Result{Graph: graph},
			flow:         transfer.Result{branch: reachable},
			boundaryXfer: func(_ transfer.NodeContext, in state.State) state.State { return in },
			edgeXfer: func(ctx transfer.EdgeContext, in state.State) state.State {
				if ctx.Edge.To == right {
					return domain.Bottom()
				}
				return in
			},
		}
		result.sealObservations()
		fallback := *result
		fallback.published = PublishedFacts{}
		if got, slow := result.EdgeCanCompleteNormally(branch, left), fallback.computeEdgeCanCompleteNormally(branch, left); got != slow || !got {
			t.Fatal("reachable branch edge cached as unreachable")
		}
		if got, slow := result.EdgeCanCompleteNormally(branch, right), fallback.computeEdgeCanCompleteNormally(branch, right); got != slow || got {
			t.Fatal("bottom branch edge cached as reachable")
		}
	})

	t.Run("nonbranch-no-normal-return", func(t *testing.T) {
		graph := cfg.New()
		call := graph.AddNode(cfg.NodeCall)
		next := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(graph.Entry(), call, false)
		graph.AddEdge(call, next, false)
		graph.AddEdge(next, graph.Exit(), false)
		domain := state.Domain(reg)
		result := &Result{
			registry:     reg,
			cfg:          &cfgbuild.Result{Graph: graph},
			facts:        factflow.NewFacts(factflow.FactsInput{NoNormalReturns: map[cfg.Point]struct{}{call: {}}}),
			flow:         transfer.Result{call: reachable},
			boundaryXfer: func(_ transfer.NodeContext, _ state.State) state.State { return domain.Bottom() },
			edgeXfer:     func(_ transfer.EdgeContext, in state.State) state.State { return in },
		}
		result.sealObservations()
		if got, ok := result.publishedNodeOutputReachable(call); !ok || got {
			t.Fatalf("node output cache=%v/%v, want cached unreachable", got, ok)
		}
		if result.EdgeCanCompleteNormally(call, next) {
			t.Fatal("no-normal-return nonbranch edge cached as reachable")
		}
	})
}

func BenchmarkResultPointReachable(b *testing.B) {
	reg := standard.Registry()
	point := cfg.Point(1)
	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(1)), product.Top())
	base := Result{registry: reg, flow: transfer.Result{point: in}}
	cached := base
	cached.published.pointReachable = map[cfg.Point]bool{point: true}
	b.Run("slow-fallback", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = base.PointReachable(point)
		}
	})
	b.Run("published-cache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cached.PointReachable(point)
		}
	})
}
