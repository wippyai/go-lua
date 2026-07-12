package factapply

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type nodePointSourceValues struct {
	value product.Value
	apply func(cfg.Point, factflow.ValueSource, state.State, func(cfg.Point) state.State)
}

func (s nodePointSourceValues) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	if s.apply != nil {
		s.apply(point, source, in, read)
	}
	return s.value, true
}

func TestConcreteNodePointCancellationRollsBackProviderMutation(t *testing.T) {
	const point cfg.Point = 17
	const target symbol.ID = 717
	reg := concreteRootTransactionRegistry(t, "test.node-point-cancel")
	domain := state.Domain(reg)
	in := fullNodePointLaneSeed(t, reg, "test.node-point-cancel")
	_, session := cancellation.Attach(context.Background())
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 1717, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "cancel-target"), source),
		},
	})
	config := FactsNodeTransferConfig{
		Facts: facts,
		Sources: nodePointSourceValues{
			value: presentValue(reg),
			apply: func(cfg.Point, factflow.ValueSource, state.State, func(cfg.Point) state.State) {
				session.Token().Cancel(context.Canceled)
			},
		},
	}
	ctx := transfer.NodeContext{Registry: reg, Point: point, Session: session}
	result := NewConcreteNodePointExecutor(config).Apply(ctx, in)
	if !result.Canceled {
		t.Fatal("provider cancellation was not reported")
	}
	if !domain.Equal(result.Output, in) {
		t.Fatal("provider cancellation published a partial node transaction")
	}
	if got := NewFactsNodeTransfer(config)(ctx, in); !domain.Equal(got, in) {
		t.Fatal("node transfer wrapper did not preserve transactional rollback")
	}
}

func TestConcreteNodePointProviderReadOrderAndLifetime(t *testing.T) {
	const point cfg.Point = 23
	const dependency cfg.Point = 3
	const target symbol.ID = 723
	reg := concreteRootTransactionRegistry(t, "test.node-point-order")
	domain := state.Domain(reg)
	in := fullNodePointLaneSeed(t, reg, "test.node-point-order")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 2323, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "ordered-target"), source),
		},
	})
	var events []string
	config := FactsNodeTransferConfig{
		Facts: facts,
		Sources: nodePointSourceValues{
			value: presentValue(reg),
			apply: func(gotPoint cfg.Point, _ factflow.ValueSource, gotInput state.State, read func(cfg.Point) state.State) {
				events = append(events, fmt.Sprintf("source:%d", gotPoint))
				if !domain.Equal(gotInput, in) {
					t.Fatal("source provider observed evolving output instead of immutable input")
				}
				_ = read(dependency)
				_ = read(dependency)
			},
		},
	}
	ctx := transfer.NodeContext{
		Registry: reg,
		Point:    point,
		Read: func(got cfg.Point) state.State {
			events = append(events, fmt.Sprintf("read:%d", got))
			return in
		},
	}
	executor := NewConcreteNodePointExecutor(config)
	for round := 0; round < 2; round++ {
		result := executor.Apply(ctx, in)
		if result.Canceled {
			t.Fatalf("round %d unexpectedly canceled", round)
		}
	}
	want := []string{"source:23", "read:3", "read:3", "source:23", "read:3", "read:3"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("provider/read events = %v, want %v", events, want)
	}
}

func TestConcreteNodePointRandomizedPreservesAllStateLanes(t *testing.T) {
	const point cfg.Point = 29
	const target symbol.ID = 729
	const userAxis userlattice.AxisID = "test.node-point-lanes"
	reg := concreteRootTransactionRegistry(t, userAxis)
	domain := state.Domain(reg)
	in := fullNodePointLaneSeed(t, reg, userAxis)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 2929, HasExpr: true}
	sources := nodePointSourceValues{value: presentValue(reg)}
	rng := rand.New(rand.NewSource(0x17a11))
	for trial := 0; trial < 400; trial++ {
		input := factflow.FactsInput{}
		hasAssignment := rng.Intn(2) == 0
		noReturn := rng.Intn(7) == 0
		if hasAssignment {
			input.RootAssignments = map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, pathdom.NewPath(target, "lane-target"), source),
			}
		}
		if noReturn {
			input.NoNormalReturns = map[cfg.Point]struct{}{point: {}}
		}
		config := FactsNodeTransferConfig{Facts: factflow.NewFacts(input), Sources: sources}
		ctx := transfer.NodeContext{Registry: reg, Point: point}
		got := NewConcreteNodePointExecutor(config).Apply(ctx, in)
		wrapped := NewFactsNodeTransfer(config)(ctx, in)
		if got.Canceled {
			t.Fatalf("trial %d unexpectedly canceled", trial)
		}
		if !domain.Equal(got.Output, wrapped) {
			t.Fatalf("trial %d wrapper and transaction differ", trial)
		}
		if noReturn {
			if !domain.Equal(got.Output, state.State{}) {
				t.Fatalf("trial %d no-return did not terminate normal state", trial)
			}
			continue
		}
		if !hasAssignment && !domain.Equal(got.Output, in) {
			t.Fatalf("trial %d empty point changed a seeded lane", trial)
		}
		// A local assignment may change Values and PathEvidence; every other
		// seeded lane must survive the complete point transaction exactly.
		for _, lane := range state.DefaultLanes() {
			if lane == state.LaneValues || lane == state.LanePathEvidence {
				continue
			}
			laneDomain := state.DomainWithLanes(reg, []state.LaneID{lane})
			if !laneDomain.Equal(got.Output, in) {
				t.Fatalf("trial %d changed unrelated lane %s", trial, lane)
			}
		}
	}
}

func fullNodePointLaneSeed(t *testing.T, reg *axis.Registry, userAxis userlattice.AxisID) state.State {
	t.Helper()
	ks := keyspace.New()
	domain := state.Domain(reg)
	out := domain.Bottom()
	for _, seed := range concreteRootTransactionLaneSeeds(t, reg, ks, userAxis) {
		out = domain.Join(out, seed)
	}
	return out
}
