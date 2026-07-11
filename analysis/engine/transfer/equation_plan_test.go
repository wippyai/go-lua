package transfer

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestTryRunOwnsNoDuplicateEquationBuilder(t *testing.T) {
	data, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "solve.EquationSystem[cfg.Point, state.State]{") {
		t.Fatal("TryRun rebuilt an equation system outside equation_plan.go")
	}
}

func TestEquationPlanCleanDifferentialAndOwnerHooks(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	left := graph.AddNode(cfg.NodeNoop)
	right := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), left, false)
	graph.AddEdge(left, right, false)
	graph.AddEdge(right, graph.Exit(), false)
	slot := key.ReturnSlot(77)
	entry := state.State{}.WriteValue(reg, slot, presentValue(reg))

	type relation struct{ owner, other cfg.Point }
	var reads, emits []relation
	ctx, session := cancellation.Attach(nil)
	stats := &Stats{}
	config := Config{Context: ctx, Session: session, Graph: graph, Registry: reg, EntryState: entry, Stats: stats}
	plan := newEquationPlan(config, state.Domain(reg), equationPlanHooks{
		read: func(owner, dependency cfg.Point, _ state.State, _ uint64, _ bool) {
			reads = append(reads, relation{owner, dependency})
		},
		emit: func(owner, destination cfg.Point, _ state.State) { emits = append(emits, relation{owner, destination}) },
	})
	planned := solve.Solve(plan.system)
	clean, err := TryRun(Config{Graph: graph, Registry: reg, EntryState: entry})
	if err != nil {
		t.Fatalf("TryRun: %v", err)
	}
	domain := state.Domain(reg)
	for _, point := range graph.RPO() {
		if !domain.Equal(planned[point], clean[point]) {
			t.Fatalf("point %d differs between canonical plan and clean TryRun", point)
		}
	}
	if len(reads) == 0 || len(emits) == 0 {
		t.Fatalf("owner hooks = %d reads, %d emits; want both", len(reads), len(emits))
	}
	for _, emission := range emits {
		declared := false
		for _, successor := range cfg.SuccessorsReadOnly(graph, emission.owner) {
			if successor == emission.other {
				declared = true
				break
			}
		}
		if !declared {
			t.Fatalf("owner %d emitted to non-successor %d", emission.owner, emission.other)
		}
	}
	if plan.solverPolicy.version != currentEquationSolverPolicyVersion || plan.identity.generation == 0 {
		t.Fatalf("plan identity/policy = %#v/%#v", plan.identity, plan.solverPolicy)
	}
	if !plan.sameIdentity(plan) {
		t.Fatal("plan does not match its own identity")
	}
	other := newEquationPlan(config, state.Domain(reg), equationPlanHooks{})
	if plan.sameIdentity(other) {
		t.Fatal("independently constructed plan reused identity")
	}
}

func TestEquationPlanCarriesExactWTOAndPolicy(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	loop := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), loop, false)
	graph.AddEdge(loop, loop, false)
	graph.AddEdge(loop, graph.Exit(), false)
	wto := solve.NewWTOPlan(graph.RPO(), func(point cfg.Point) []cfg.Point { return cfg.SuccessorsReadOnly(graph, point) })
	ctx, session := cancellation.Attach(nil)
	delay := func(point cfg.Point) int {
		if point == loop {
			return 3
		}
		return 0
	}
	plan := newEquationPlan(Config{Context: ctx, Session: session, Graph: graph, Registry: reg, WTOPlan: wto, WidenDelay: delay}, state.Domain(reg), equationPlanHooks{})
	if plan.wto != wto || plan.system.WidenDelay(loop) != 3 || plan.system.WidenAt == nil {
		t.Fatalf("plan lost WTO/widen policy: wto=%v delay=%d widen=%v", plan.wto == wto, plan.system.WidenDelay(loop), plan.system.WidenAt != nil)
	}
	if plan.system.Abstract != nil {
		t.Fatal("transfer clean equation unexpectedly installed Abstract")
	}
}

func TestEquationPlanVersionedObservationPreservesEdgeReadContract(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)
	ctx, session := cancellation.Attach(nil)
	versionedReads := 0
	edges := 0
	plan := newEquationPlan(Config{
		Context: ctx, Session: session, Graph: graph, Registry: reg,
		ObserveNode:           func(cfg.Point) bool { return true },
		RecordNodeObservation: func(NodeObservation) {},
		EdgeTransfer: func(edge EdgeContext, out state.State) state.State {
			edges++
			if edge.Read != nil {
				t.Fatal("versioned observation exposed EdgeContext.Read; clean behavior requires nil")
			}
			return out
		},
	}, state.Domain(reg), equationPlanHooks{
		read: func(_, _ cfg.Point, _ state.State, _ uint64, versioned bool) {
			if versioned {
				versionedReads++
			}
		},
	})
	if plan.system.TransferVersioned == nil {
		t.Fatal("observing plan omitted versioned transfer")
	}
	_, _ = solve.SolveWithVersions(plan.system)
	if edges == 0 || versionedReads == 0 {
		t.Fatalf("versioned execution = %d edges, %d reads", edges, versionedReads)
	}
}
