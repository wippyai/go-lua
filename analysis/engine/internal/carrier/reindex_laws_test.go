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

func TestRuntimeReindexSealsOnlyOverExistingScopesAfterWorkOpens(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	empty, ok := composition.SealScope(nil)
	if !ok {
		t.Fatal("empty scope")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()

	if _, ok := composition.NewReindex(composition.Scope(), empty); ok {
		t.Fatal("ordinary cold reindex reopened after work")
	}
	if _, ok := composition.SealScope([]guard.Atom{1}); ok {
		t.Fatal("runtime reindex path reopened scope sealing")
	}
	builder, ok := composition.NewRuntimeReindex(composition.Scope(), empty)
	if !ok {
		t.Fatal("runtime reindex over issued scopes")
	}
	if builder.Identity(1) {
		t.Fatal("runtime reindex admitted identity outside target scope")
	}
	if !builder.Forget(1) {
		t.Fatal("runtime reindex forgot existing source atom")
	}
	plan, ok := builder.Seal()
	if !ok || !plan.Valid() || plan.CoordinateIdentity() {
		t.Fatal("runtime reindex did not seal immutable forget relation")
	}

	foreign, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("foreign composition")
	}
	if _, ok := composition.NewRuntimeReindex(foreign.Scope(), empty); ok {
		t.Fatal("runtime reindex admitted foreign issued scope")
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
	contribution, ok := composition.SealContribution(2, []shape.Slot{0}, nil)
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
	if _, ok := work.BeginRuleContribution(contribution, composition.Scope(), contributionPoints(t, work, state, reindexed), whole); ok {
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

// TestScopeValidIsSealedAtConstruction pins that Scope.Valid reads the
// completeness verdict newScope reached once, at construction, rather than
// re-deriving it from composition/guard on every call.  Detaching those
// fields from an already-issued scope therefore cannot flip the verdict, and
// the read allocates nothing.
func TestScopeValidIsSealedAtConstruction(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	scope, ok := composition.SealScope([]guard.Atom{1})
	if !ok || !scope.Valid() {
		t.Fatal("issued scope unavailable")
	}
	detached := scope
	detached.guard = guard.Scope{}
	if !detached.Valid() {
		t.Fatal("Valid re-derives from guard instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = scope.Valid() }); allocs != 0 {
		t.Fatalf("Valid allocates %v per call", allocs)
	}
	if (Scope{}).Valid() {
		t.Fatal("zero scope available")
	}
}

// TestScopeRefusesMalformedConstruction pins that SealScope is the sole
// authenticator: an atom outside the composition's guard universe never
// reaches a published Scope.
func TestScopeRefusesMalformedConstruction(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	if scope, ok := composition.SealScope([]guard.Atom{99}); ok || scope.Valid() {
		t.Fatal("unowned atom sealed into a published scope")
	}
	var nilComposition *Composition
	if scope, ok := nilComposition.SealScope([]guard.Atom{1}); ok || scope.Valid() {
		t.Fatal("nil-composition scope sealed")
	}
}

// TestReindexPlanValidIsSealedAtConstruction pins that ReindexPlan.Valid reads
// the completeness verdict newReindexPlan reached once, at construction,
// rather than re-deriving it from composition/relation on every call.
// Detaching those fields from an already-sealed plan therefore cannot flip
// the verdict, and the read allocates nothing.
func TestReindexPlanValidIsSealedAtConstruction(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	plan, ok := composition.IdentityReindex(composition.Scope())
	if !ok || !plan.Valid() {
		t.Fatal("issued reindex plan unavailable")
	}
	detached := plan
	detached.relation = guard.Reindex{}
	if !detached.Valid() {
		t.Fatal("Valid re-derives from relation instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = plan.Valid() }); allocs != 0 {
		t.Fatalf("Valid allocates %v per call", allocs)
	}
	if (ReindexPlan{}).Valid() {
		t.Fatal("zero reindex plan available")
	}
}

// TestReindexPlanRefusesMalformedConstruction pins that Seal is the sole
// authenticator: an incomplete source-coordinate assignment never reaches a
// published ReindexPlan.
func TestReindexPlanRefusesMalformedConstruction(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	source, ok := composition.SealScope([]guard.Atom{1, 2})
	if !ok {
		t.Fatal("source scope")
	}
	builder, ok := composition.NewReindex(source, composition.Scope())
	if !ok || !builder.Identity(1) {
		t.Fatal("partial reindex construction")
	}
	// atom 2 is left unassigned: Seal must reject an incomplete relation.
	if plan, ok := builder.Seal(); ok || plan.Valid() {
		t.Fatal("incomplete source relation sealed")
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
