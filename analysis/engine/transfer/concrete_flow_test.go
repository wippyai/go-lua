package transfer

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/solve/concreteflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestConcreteFlowRandomReducibleDifferentialAllLanes(t *testing.T) {
	reg := standard.Registry()
	rng := rand.New(rand.NewSource(0xD35E))
	selections := [][]state.LaneID{nil}
	for _, lane := range state.DefaultLanes() {
		selections = append(selections, []state.LaneID{lane})
	}
	if len(state.DefaultLanes()) != 17 {
		t.Fatalf("default lanes=%d, want 17", len(state.DefaultLanes()))
	}
	for trial := 0; trial < 120; trial++ {
		graph := cfg.New()
		count := 4 + rng.Intn(20)
		points := make([]cfg.Point, count)
		for i := range points {
			points[i] = graph.AddNode(cfg.NodeAssign)
		}
		graph.AddEdge(graph.Entry(), points[0], false)
		for i := 0; i+1 < len(points); i++ {
			graph.AddEdge(points[i], points[i+1], false)
		}
		graph.AddEdge(points[len(points)-1], graph.Exit(), false)
		// Backedges over a linear dominator chain create reducible, frequently
		// nested SCCs while keeping the expected solution easy to compare.
		for i := 1; i < len(points); i++ {
			if rng.Intn(4) == 0 {
				head := rng.Intn(i)
				graph.AddEdge(points[i], points[head], false)
			}
		}
		wto := solve.NewWTOPlan(graph.RPO(), func(point cfg.Point) []cfg.Point {
			return cfg.SuccessorsReadOnly(graph, point)
		})
		dense, err := concreteflow.Compile(graph, operationplan.New(graph, factflow.FactsInput{}), wto)
		if err != nil {
			t.Fatalf("trial %d compile: %v", trial, err)
		}
		node := func(ctx NodeContext, in state.State) state.State {
			return in.WriteValue(reg, statekey.ReturnSlot(int(ctx.Point%32)), product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
		}
		for _, lanes := range selections {
			base := Config{Graph: graph, Registry: reg, Schedule: ScheduleWTO, WTOPlan: wto, StateLanes: lanes, NodeTransfer: node}
			want, err := TryRun(base)
			if err != nil {
				t.Fatalf("trial %d lanes %v generic: %v", trial, lanes, err)
			}
			base.ConcreteFlow = dense
			got, err := TryRun(base)
			if err != nil {
				t.Fatalf("trial %d lanes %v dense: %v", trial, lanes, err)
			}
			domain := state.DomainWithOptionalLanes(reg, lanes)
			for _, point := range graph.RPO() {
				if !domain.Equal(want[point], got[point]) {
					t.Fatalf("trial %d lanes %v point %d differs", trial, lanes, point)
				}
			}
		}
	}
}

func TestConcreteFlowCancellationDiscardsScratch(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	wto := solve.NewWTOPlan(graph.RPO(), func(p cfg.Point) []cfg.Point { return cfg.SuccessorsReadOnly(graph, p) })
	plan, err := concreteflow.Compile(graph, operationplan.New(graph, factflow.FactsInput{}), wto)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result, err := TryRun(Config{Context: ctx, Graph: graph, Registry: reg, Schedule: ScheduleWTO, WTOPlan: wto, ConcreteFlow: plan,
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point == point {
				cancel()
			}
			return in
		}})
	if result != nil {
		t.Fatal("canceled dense solve published a result")
	}
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want cancellation", err)
	}
}
