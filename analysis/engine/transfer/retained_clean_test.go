package transfer

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func retainedFixture(t *testing.T) (Config, cfg.Graph) {
	t.Helper()
	reg := standard.Registry()
	graph := cfg.New()
	before := graph.AddNode(cfg.NodeNoop)
	head := graph.AddNode(cfg.NodeNoop)
	body := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), before, false)
	graph.AddEdge(before, head, false)
	graph.AddEdge(head, body, false)
	graph.AddEdge(body, head, false)
	graph.AddEdge(head, graph.Exit(), false)
	plan := solve.NewWTOPlan(graph.RPO(), func(point cfg.Point) []cfg.Point { return cfg.SuccessorsReadOnly(graph, point) })
	first := key.ReturnSlot(91)
	second := key.ReturnSlot(92)
	entry := state.State{}.WriteValue(reg, first, presentValue(reg))
	config := Config{
		Graph: graph, Registry: reg, EntryState: entry, Schedule: ScheduleWTO, WTOPlan: plan,
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point == before {
				return in.WriteValue(reg, second, presentValue(reg))
			}
			if ctx.Point == graph.Exit() {
				_ = ctx.Read(graph.Entry())
			}
			return in
		},
		EdgeTransfer: func(ctx EdgeContext, out state.State) state.State {
			// Ordinary clean semantics expose dynamic reads to edge transfer. The
			// retained versioned adapter must preserve this exact contract.
			if ctx.Edge.From == head {
				_ = ctx.Read(graph.Entry())
			}
			return out
		},
	}
	return config, graph
}

func TestRetainedCleanMatchesOrdinaryWTOStatesAndVersions(t *testing.T) {
	config, graph := retainedFixture(t)
	ordinaryConfig := config
	ordinaryConfig.Context, ordinaryConfig.Session = cancellation.Attach(nil)
	domain := state.Domain(config.Registry)
	ordinaryPlan := newEquationPlan(ordinaryConfig, domain, equationPlanHooks{})
	want, wantVersions, err := solve.SolveWTOContextWithVersions(ordinaryConfig.Context, ordinaryPlan.system, ordinaryPlan.wto)
	if err != nil {
		t.Fatal(err)
	}

	got, generation, err := tryRunRetainedClean(config, retainedBudget{MaxOwners: 32, MaxReads: 64, MaxOutputs: 64, MaxStateRefs: 256})
	if err != nil {
		t.Fatal(err)
	}
	defer generation.Release()
	for _, point := range graph.RPO() {
		if !domain.Equal(got[point], want[point]) {
			t.Fatalf("point %d state differs", point)
		}
		if generation.finalVersions[point] != wantVersions[point] {
			t.Fatalf("point %d version = %d, want %d", point, generation.finalVersions[point], wantVersions[point])
		}
		if _, ok := generation.retained.Value(point); !ok {
			t.Fatalf("checkpoint omitted point %d", point)
		}
	}
	if usage := generation.retained.Usage(); usage.Owners == 0 || usage.Reads == 0 || usage.Outputs == 0 || usage.StateRefs == 0 {
		t.Fatalf("incomplete retained usage: %#v", usage)
	}
}

func TestRetainedCleanBudgetAndCancellationPublishNothing(t *testing.T) {
	config, _ := retainedFixture(t)
	result, generation, err := tryRunRetainedClean(config, retainedBudget{MaxOutputs: 1})
	if !errors.Is(err, solve.ErrRetainedBudget) || result != nil || generation != nil {
		t.Fatalf("budget result=%v generation=%v err=%v", result != nil, generation != nil, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelConfig := config
	cancelConfig.Context = ctx
	baseTransfer := cancelConfig.NodeTransfer
	cancelConfig.NodeTransfer = func(node NodeContext, in state.State) state.State { cancel(); return baseTransfer(node, in) }
	result, generation, err = tryRunRetainedClean(cancelConfig, retainedBudget{})
	if !errors.Is(err, solve.ErrCanceled) || result != nil || generation != nil {
		t.Fatalf("cancel result=%v generation=%v err=%v", result != nil, generation != nil, err)
	}
}

func TestRetainedCleanReleaseDropsOwnedReferences(t *testing.T) {
	config, _ := retainedFixture(t)
	_, generation, err := tryRunRetainedClean(config, retainedBudget{})
	if err != nil {
		t.Fatal(err)
	}
	generation.Release()
	if generation.plan.system.Transfer != nil || generation.retained != nil || generation.finalVersions != nil || !generation.released {
		t.Fatalf("generation retained data after release: %#v", generation)
	}
	// Idempotence is part of ownership: a deferred release may follow an early one.
	generation.Release()
}

func BenchmarkRetainedCleanOptIn(b *testing.B) {
	reg := standard.Registry()
	graph := cfg.New()
	previous := graph.Entry()
	for range 32 {
		point := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(previous, point, false)
		previous = point
	}
	graph.AddEdge(previous, graph.Exit(), false)
	wto := solve.NewWTOPlan(graph.RPO(), func(point cfg.Point) []cfg.Point { return cfg.SuccessorsReadOnly(graph, point) })
	config := Config{Graph: graph, Registry: reg, Schedule: ScheduleWTO, WTOPlan: wto}
	b.Run("default", func(b *testing.B) {
		for range b.N {
			if _, err := TryRun(config); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("retained", func(b *testing.B) {
		for range b.N {
			_, generation, err := tryRunRetainedClean(config, retainedBudget{})
			if err != nil {
				b.Fatal(err)
			}
			generation.Release()
		}
	})
}
