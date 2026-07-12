package factapply

import (
	"context"
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestConcreteBranchEdgePointMixedRowsMatchPreparedTransferAcrossAllStateLanes(t *testing.T) {
	const userAxis userlattice.AxisID = "test.branch-edge-transaction"
	reg := concreteRootTransactionRegistry(t, userAxis)
	point := cfg.Point(901)
	to := cfg.Point(902)
	const (
		trigger = symbol.ID(901)
		target  = symbol.ID(902)
		other   = symbol.ID(903)
		array   = symbol.ID(904)
		index   = symbol.ID(905)
	)
	resolver := branchEdgeTransactionResolver(point, trigger, target, other, array, index)
	ks := resolver.KeySpace()
	triggerPath := pathdom.NewPath(trigger, "trigger")
	targetPath := pathdom.NewPath(target, "target")
	otherPath := pathdom.NewPath(other, "other")
	arrayPath := pathdom.NewPath(array, "array")
	indexPath := pathdom.NewPath(index, "index")
	rows := factflow.NewBranchRefinementSet(
		factflow.NewBranchRefinement(targetPath, factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())), true, factflow.ValueRefinement{}, false),
	).
		WithLenRefinements(factflow.NewBranchLenRefinementOnEdge(arrayPath, 4, true)).
		WithNumFloorRefinements(factflow.NewBranchNumFloorRefinementOnEdge(indexPath, 1, true)).
		WithNumCeilRefinements(factflow.NewBranchNumCeilRefinementOnEdge(indexPath, 4, true)).
		WithDiffConstraints(factflow.NewBranchScaledConstraintOnEdge(1, indexPath, false, 0, pathdom.Path{}, false, arrayPath, true, 0, true))
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{point: rows},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			point: factflow.NewBranchPathRelationSet(factflow.NewBranchPathEquality(targetPath, otherPath, true, false)),
		},
		BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
			point: factflow.NewBranchPathEvidenceSet(
				factflow.NewBranchPathPresenceEvidenceOnEdge(targetPath, presence.Present(), true),
				factflow.NewBranchPathEqualityEvidenceOnEdge(targetPath, otherPath, true),
				factflow.NewBranchIndexInRangeEvidenceOnEdge(indexPath, arrayPath, true),
			),
		},
	})
	ctx := transfer.EdgeContext{
		Registry: reg,
		Edge:     cfg.Edge{From: point, To: to, Cond: true},
		HasCond:  true,
	}
	config := FactsEdgeTransferConfig{Facts: facts, Visibility: resolver}
	prepared := NewFactsEdgeTransfer(config)
	laneSeeds := concreteRootTransactionLaneSeeds(t, reg, keyspace.New(), userAxis)
	if got := len(state.DefaultLanes()); got != 17 {
		t.Fatalf("default state lane count = %d, want 17", got)
	}

	seedBase := func() state.State {
		maybe := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
		present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
		return state.Reachable(state.State{}).
			WriteValue(reg, key.SymbolValue(trigger), present).
			WriteValue(reg, key.SymbolValue(target), maybe).
			WriteValue(reg, key.SymbolValue(other), present).
			AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
				ks.FromPath(triggerPath), presence.Present(), ks.FromPath(targetPath), presence.Present(),
			))
	}
	assertEqual := func(input state.State) {
		t.Helper()
		want := prepared(ctx, input)
		got := ApplyConcreteBranchEdgePoint(ConcreteBranchEdgePointRequest{
			Context: ctx, Facts: facts, Resolver: resolver, Output: input,
		})
		if got.Canceled {
			t.Fatal("live transaction reported cancellation")
		}
		if !state.Domain(reg).Equal(got.Output, want) {
			t.Fatal("extracted branch-edge transaction differs from prepared transfer")
		}
	}
	for _, lane := range state.DefaultLanes() {
		assertEqual(state.Domain(reg).Join(seedBase(), laneSeeds[lane]))
	}
	rng := rand.New(rand.NewSource(0xb4a2c4))
	for iteration := 0; iteration < 512; iteration++ {
		input := seedBase()
		for _, lane := range state.DefaultLanes() {
			if rng.Intn(2) == 0 {
				input = state.Domain(reg).Join(input, laneSeeds[lane])
			}
		}
		assertEqual(input)
	}
}

func TestConcreteBranchEdgePointCancellationKeepsEvolvingOutput(t *testing.T) {
	reg := concreteRootTransactionRegistry(t, "test.branch-edge-cancel")
	point := cfg.Point(911)
	resolver := branchEdgeTransactionResolver(point, 911)
	evolving := state.Reachable(state.State{}).WriteValue(reg, key.SymbolValue(911), presentValue(reg))
	ctx, cancel := context.WithCancel(context.Background())
	_, session := cancellation.Attach(ctx)
	cancel()
	<-ctx.Done()
	result := ApplyConcreteBranchEdgePoint(ConcreteBranchEdgePointRequest{
		Context: transfer.EdgeContext{
			Registry: reg, Edge: cfg.Edge{From: point, To: point + 1, Cond: true},
			HasCond: true, Session: session,
		},
		Facts: factflow.NewFacts(factflow.FactsInput{}), Resolver: resolver, Output: evolving,
	})
	if !result.Canceled {
		t.Fatal("canceled branch-edge transaction did not report cancellation")
	}
	if !state.Domain(reg).Equal(result.Output, evolving) {
		t.Fatal("canceled branch-edge transaction rolled back its evolving output")
	}
}

func branchEdgeTransactionResolver(point cfg.Point, symbols ...symbol.ID) *visibility.Resolver {
	builder := visibility.NewBuilder()
	for _, sym := range symbols {
		builder.Define(point, sym, "branch-edge")
	}
	return visibility.NewResolver(builder.Build())
}
