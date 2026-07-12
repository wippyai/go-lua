package factapply

import (
	"context"
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestConcreteGenericForRandomizedMatchesLegacyCompositionAcrossAllLanes(t *testing.T) {
	const (
		point  = cfg.Point(711)
		target = symbol.ID(711)
		first  = symbol.ID(712)
	)
	userAxis := userlattice.AxisID("test.generic-for")
	reg := concreteRootTransactionRegistry(t, userAxis)
	resolver := branchEdgeTransactionResolver(point, target, first)
	op, ok := NewGenericForOperation(1, target, first, GenericForSource{Kind: GenericForSourceCall, CallPoint: 17, HasCallPoint: true}, nil)
	if !ok {
		t.Fatal("NewGenericForOperation rejected valid operation")
	}
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	targetPath := pathdom.Path{Symbol: target}
	targetKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	if !ok {
		t.Fatal("target state key unavailable")
	}
	firstKey, ok := visibility.AddressAt(resolver, point, pathdom.Path{Symbol: first}).VisibleStateKey()
	if !ok {
		t.Fatal("first state key unavailable")
	}
	seeds := concreteRootTransactionLaneSeeds(t, reg, resolver.KeySpace(), userAxis)
	all := state.State{}
	domain := state.Domain(reg)
	for _, seed := range seeds {
		all = domain.Join(all, seed)
	}
	values := make([]state.State, 0, len(seeds)+1)
	values = append(values, all)
	for _, seed := range seeds {
		values = append(values, seed)
	}
	rng := rand.New(rand.NewSource(711))
	for i := 0; i < 300; i++ {
		input := values[rng.Intn(len(values))]
		output := values[rng.Intn(len(values))]
		resolved := rng.Intn(2) == 0
		value := presentValue(reg)
		resolve := func(transfer.NodeContext, GenericForOperation, state.State) (product.Value, bool) {
			return value, resolved
		}
		membership := func(_ transfer.NodeContext, _ GenericForOperation, out state.State, _ pathdom.Path) state.State {
			return out.AddPathKeyMembership(targetKey, firstKey)
		}

		want := output.ClearKeyMembershipsForPath(targetKey)
		if resolved {
			want = want.WriteValue(reg, key.SymbolValue(target), value)
		}
		want = membership(ctx, op, want, targetPath)
		got := ApplyConcreteGenericFor(ConcreteGenericForRequest{
			Context: ctx, Resolver: resolver, Input: input, Output: output,
			Operation: op, Semantics: genericForTestSemantics{resolve: resolve, membership: membership},
		})
		if got.Canceled || !domain.Equal(got.Output, want) {
			t.Fatalf("iteration %d differs from legacy composition", i)
		}
	}
}

func TestConcreteGenericForCancellationRollsBackProviderPrefix(t *testing.T) {
	reg := concreteRootTransactionRegistry(t, "test.generic-for-cancel")
	resolver := branchEdgeTransactionResolver(721, 721)
	op, _ := NewGenericForOperation(0, 721, 721, GenericForSource{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	ctx, session := cancellation.Attach(ctx)
	base := state.State{}.WriteValue(reg, key.SymbolValue(999), presentValue(reg))
	result := ApplyConcreteGenericFor(ConcreteGenericForRequest{
		Context:  transfer.NodeContext{Context: ctx, Session: session, Registry: reg, Point: 721},
		Resolver: resolver, Input: base, Output: base, Operation: op,
		Semantics: genericForTestSemantics{resolve: func(transfer.NodeContext, GenericForOperation, state.State) (product.Value, bool) {
			cancel()
			return presentValue(reg), true
		}},
	})
	if !result.Canceled || !state.Domain(reg).Equal(result.Output, base) {
		t.Fatal("canceled transaction published a partial generic-for write")
	}
}

type genericForTestSemantics struct {
	resolve    func(transfer.NodeContext, GenericForOperation, state.State) (product.Value, bool)
	membership func(transfer.NodeContext, GenericForOperation, state.State, pathdom.Path) state.State
}

func (s genericForTestSemantics) ResolveGenericFor(ctx transfer.NodeContext, op GenericForOperation, in state.State) (product.Value, bool) {
	if s.resolve == nil {
		return product.Value{}, false
	}
	return s.resolve(ctx, op, in)
}

func (s genericForTestSemantics) ApplyGenericForMembership(ctx transfer.NodeContext, op GenericForOperation, out state.State, path pathdom.Path) state.State {
	if s.membership == nil {
		return out
	}
	return s.membership(ctx, op, out, path)
}
