package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestReindexIdentityReusesExactStateAndRootVector(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	plan, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity plan")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	next, ok := work.Reindex(state, plan)
	if !ok || !sameState(next, state) || !next.Scope().same(state.Scope()) {
		t.Fatalf("identity reindex = ok:%t same:%t scope:%t", ok, sameState(next, state), next.Scope().same(state.Scope()))
	}
}

func TestReindexRejectsPlanWhoseSourceScopeDoesNotMatchState(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	empty, ok := composition.SealScope(nil)
	if !ok {
		t.Fatal("empty scope")
	}
	builder, ok := composition.NewReindex(empty, empty)
	if !ok {
		t.Fatal("builder")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("plan")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	before, _ := state.HandleAt(0)
	if _, reindexed := work.Reindex(state, plan); reindexed {
		t.Fatal("mixed-source plan reindexed")
	}
	after, _ := state.HandleAt(0)
	if before != after || !state.Support().SameHandle(whole) {
		t.Fatal("rejected reindex changed predecessor")
	}
}

func TestScopedStatesRejectMixedEqualityOrderMergeReplaceAndContribution(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	target, ok := composition.SealScope([]guard.Atom{1})
	if !ok {
		t.Fatal("target scope")
	}
	builder, ok := composition.NewReindex(composition.Scope(), target)
	if !ok || !builder.Identity(1) {
		t.Fatal("identity-shaped builder")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("identity-shaped plan")
	}
	contribution, ok := composition.SealContribution(2, []shape.Slot{0}, nil, false)
	if !ok {
		t.Fatal("contribution")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	view, ok := state.Restrict(whole)
	if !ok || !view.state.Scope().same(state.Scope()) {
		t.Fatal("restriction did not preserve State scope")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	reindexed, ok := work.Reindex(state, plan)
	if !ok || reindexed.Scope().same(state.Scope()) {
		t.Fatal("distinct target scope was not retained")
	}
	if work.EqualUnder(state, reindexed) || work.LessOrEqUnder(state, reindexed) {
		t.Fatal("mixed scopes entered equality or order")
	}
	if _, _, ok := work.Merge3Under(Join, state, reindexed, composition.AllMergeScope()); ok {
		t.Fatal("mixed scopes merged")
	}
	if _, _, ok := work.Replace(state, reindexed); ok {
		t.Fatal("mixed scopes replaced")
	}
	if _, ok := work.BeginContribution(contribution, composition.Scope(), contributionInputs(t, work, state, reindexed), whole); ok {
		t.Fatal("mixed scopes entered contribution")
	}
}

func TestNewStateAcceptsFeasibleSupportWithinStrictIssuedScope(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	strict, ok := composition.SealScope([]guard.Atom{1})
	if !ok {
		t.Fatal("strict scope")
	}
	build := support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	within, ok := build.Literal(1, true)
	if !ok || !build.Seal() {
		t.Fatal("strict support")
	}
	state, ok := NewState(composition, strict, within)
	if !ok || !state.Valid() || !state.Scope().Same(strict) {
		t.Fatalf("strict scope state = ok:%t valid:%t scope:%t", ok, state.Valid(), state.Scope().Same(strict))
	}
}

func TestNewStateRejectsFeasibleSupportOutsideIssuedScope(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	strict, ok := composition.SealScope([]guard.Atom{1})
	if !ok {
		t.Fatal("strict scope")
	}
	build := support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	outside, ok := build.Literal(2, true)
	if !ok || !build.Seal() {
		t.Fatal("outside support")
	}
	if _, accepted := NewState(composition, strict, outside); accepted {
		t.Fatal("accepted support that mentions an out-of-scope coordinate")
	}
}

func TestNewStateRejectsScopeIssuedByForeignComposition(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	foreign, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("foreign composition")
	}
	if _, accepted := NewState(composition, foreign.Scope(), whole); accepted {
		t.Fatal("accepted scope issued by another composition")
	}
}

func TestNewStateAcceptsEmptyIssuedScopeWithConstantSupport(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	empty, ok := composition.SealScope(nil)
	if !ok {
		t.Fatal("empty scope")
	}
	state, ok := NewState(composition, empty, whole)
	if !ok || !state.Valid() || !state.Scope().Same(empty) {
		t.Fatalf("empty scope state = ok:%t valid:%t scope:%t", ok, state.Valid(), state.Scope().Same(empty))
	}
}
