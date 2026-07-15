package concreteflow

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRunNarrowingParityBeyondDepth64(t *testing.T) {
	const depth = 70
	graph := cfg.New()
	previous := graph.Entry()
	for i := 0; i < depth; i++ {
		point := graph.AddNode(cfg.NodeAssign)
		graph.AddEdge(previous, point, false)
		previous = point
	}
	graph.AddEdge(previous, graph.Exit(), false)
	cells := graph.RPO()
	wto := solve.NewWTOPlan(cells, func(point cfg.Point) []cfg.Point {
		return cfg.SuccessorsReadOnly(graph, point)
	})
	plan, err := Compile(graph, operationplan.New(graph, factflow.FactsInput{}), wto)
	if err != nil {
		t.Fatal(err)
	}

	reg := standard.Registry()
	ks := keyspace.New()
	root := pathdom.NewPath(symbol.ID(1), "n")
	root.Version = 1
	stateKey, ok := pathaddr.StateKeyFromPathKey(root.Key())
	if !ok {
		t.Fatal("failed to construct numeric state key")
	}
	domain := state.DomainWithLanes(reg, []state.LaneID{state.LaneNumCeils})
	exact := domain.Bottom().WriteNumCeil(ks, stateKey, 7)
	probe := domain.Narrow(domain.Top(), exact)
	if !domain.LessOrEq(probe, domain.Top()) || domain.Equal(probe, domain.Top()) {
		t.Fatalf("num-ceil narrowing does not strictly descend from top: %#v", probe.NumCeilsSnapshot(ks))
	}

	newSystem := func(rounds *int) solve.EquationSystem[cfg.Point, state.State] {
		return solve.EquationSystem[cfg.Point, state.State]{
			Lattice: domain,
			Cells:   cells,
			InitialSparse: func(point cfg.Point) (state.State, bool) {
				return exact, point == graph.Entry()
			},
			Transfer: func(point cfg.Point, read func(cfg.Point) state.State, emit func(cfg.Point, state.State)) {
				if point == graph.Entry() {
					(*rounds)++
				}
				value := read(point)
				for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
					emit(successor, value)
				}
			},
			TransferVersioned: func(point cfg.Point, _ func(cfg.Point) (state.State, uint64), emit func(cfg.Point, state.State)) {
				for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
					emit(successor, domain.Top())
				}
			},
			WidenAt: func(cfg.Point) bool { return true },
		}
	}

	genericRounds := 0
	want := solve.Solve(newSystem(&genericRounds))
	denseRounds := 0
	got, err := Run(RunConfig{Context: context.Background()}, newSystem(&denseRounds), plan)
	if err != nil {
		t.Fatal(err)
	}
	if genericRounds <= 64 || denseRounds <= 64 {
		t.Fatalf("narrowing rounds generic/dense = %d/%d, want both beyond 64", genericRounds, denseRounds)
	}
	for _, point := range cells {
		if !domain.Equal(want[point], got.Points[point]) {
			t.Fatalf("point %d differs between generic and concreteflow", point)
		}
		if ceil, ok := got.Points[point].ReadNumCeil(ks, stateKey); !ok || ceil != 7 {
			t.Fatalf("point %d ceil = %d/%v, want exact 7", point, ceil, ok)
		}
	}
}
