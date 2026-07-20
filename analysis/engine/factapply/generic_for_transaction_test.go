package factapply

import (
	"context"
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestGenericForRandomizedMatchesCanonicalTransactionAcrossAllLanes(t *testing.T) {
	const (
		point  = cfg.Point(711)
		target = symbol.ID(711)
		first  = symbol.ID(712)
	)
	userAxis := userlattice.AxisID("test.generic-for")
	reg := concreteRootTransactionRegistry(t, userAxis)
	resolver := branchEdgeTransactionResolver(point, target, first)
	op, ok := NewGenericForOperation(1, target, first, []GenericForSource{{Kind: GenericForSourceCall, CallPoint: 17, HasCallPoint: true}}, nil)
	if !ok {
		t.Fatal("NewGenericForOperation rejected valid operation")
	}
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	targetPath := pathdom.Path{Symbol: target}
	targetKey, ok := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	if !ok {
		t.Fatal("target state key unavailable")
	}
	targetStructural, ok := visibility.AddressAt(resolver, point, targetPath).VisibleLocalKeyspaceKey()
	if !ok {
		t.Fatal("target structural key unavailable")
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
		want := output.ClearKeyMembershipsForPath(targetKey)
		if resolved {
			want = want.WriteValue(reg, key.SymbolValue(target), value)
		}
		productDomain := state.RegisteredProductDomain(reg)
		got := ApplyGenericFor(GenericForRequest{
			Context: ctx, Input: input, Output: output, Operation: op,
			ResolvedValue: value, HasResolvedValue: resolved,
			Domain: productDomain,
			Membership: genericForTestMembership{config: state.GenericForFactorConfig{
				Keys: resolver.KeySpace(), VariableIndex: op.VariableIndex(), Target: targetStructural,
			}},
		})
		if got.Canceled || !domain.Equal(got.Output, want) {
			t.Fatalf("iteration %d differs from canonical composition", i)
		}
	}
}

func TestGenericForCancellationRollsBackMembershipPrefix(t *testing.T) {
	reg := concreteRootTransactionRegistry(t, "test.generic-for-cancel")
	op, _ := NewGenericForOperation(0, 721, 721, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	ctx, session := cancellation.Attach(ctx)
	cancel()
	base := state.State{}.WriteValue(reg, key.SymbolValue(999), presentValue(reg))
	resolver := branchEdgeTransactionResolver(721, 721)
	target, _ := visibility.AddressAt(resolver, 721, pathdom.Path{Symbol: 721}).VisibleLocalKeyspaceKey()
	result := ApplyGenericFor(GenericForRequest{
		Context: transfer.NodeContext{Context: ctx, Session: session, Registry: reg, Point: 721},
		Input:   base, Output: base, Operation: op, ResolvedValue: presentValue(reg), HasResolvedValue: true,
		Domain: state.RegisteredProductDomain(reg),
		Membership: genericForTestMembership{config: state.GenericForFactorConfig{
			Keys: resolver.KeySpace(), VariableIndex: 0, Target: target,
		}},
	})
	if !result.Canceled || !state.Domain(reg).Equal(result.Output, base) {
		t.Fatal("canceled transaction published a partial generic-for write")
	}
}

type genericForTestMembership struct {
	config state.GenericForFactorConfig
}

func (s genericForTestMembership) PrepareGenericForFactorTransaction(_ transfer.NodeContext, _ GenericForOperation, domain state.ProductDomain) (state.GenericForFactorTransaction, error) {
	return domain.PrepareGenericForFactorTransaction(s.config)
}
