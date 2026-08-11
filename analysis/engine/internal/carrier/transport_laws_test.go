package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func transportFixture(t testing.TB) (*Composition, *Work, State, support.Mask, support.Mask, ReindexPlan) {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("composition")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	build := support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	trueBranch, ok := build.Literal(1, true)
	if !ok || !build.Seal() {
		t.Fatal("true branch")
	}
	build = support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	falseBranch, ok := build.Literal(1, false)
	if !ok || !build.Seal() {
		t.Fatal("false branch")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	identity, ok := composition.IdentityReindex(composition.Scope())
	if !ok {
		t.Fatal("identity")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	return composition, work, state, trueBranch, falseBranch, identity
}

func TestTransportAppliesPreThenReindexThenPost(t *testing.T) {
	composition, work, state, trueBranch, falseBranch, identity := transportFixture(t)
	defer work.Close()
	whole, ok := support.True(composition.Guards())
	if !ok {
		t.Fatal("whole")
	}
	build := support.New(composition.Guards())
	if build == nil {
		t.Fatal("support work")
	}
	empty := build.False()
	if !build.Seal() {
		t.Fatal("empty support")
	}
	identityState, ok := work.Transport(state, whole, identity, whole)
	if !ok || !sameState(identityState, state) {
		t.Fatal("exact identity did not reuse immutable State")
	}
	filtered, ok := work.Transport(state, trueBranch, identity, falseBranch)
	if !ok || !filtered.Support().Equal(empty) {
		t.Fatal("post was not applied after pre and identity transport")
	}
	before, beforeOK := state.HandleAt(0)
	after, afterOK := filtered.HandleAt(0)
	if !beforeOK || !afterOK || before != after {
		t.Fatal("filter-only transport rebuilt a plane root")
	}
	contradiction, ok := work.Transport(state, empty, identity, trueBranch)
	if !ok {
		t.Fatal("false pre transport")
	}
	if !contradiction.Support().Equal(empty) {
		t.Fatal("false precondition was not retained before transport")
	}
}

func TestTransportForgetCannotReuseSourceScope(t *testing.T) {
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
		t.Fatal("target Scope")
	}
	builder, ok := composition.NewReindex(composition.Scope(), empty)
	if !ok || !builder.Forget(1) {
		t.Fatal("forget")
	}
	forget, ok := builder.Seal()
	if !ok {
		t.Fatal("sealed forget")
	}
	whole, ok := support.True(composition.Guards())
	if !ok {
		t.Fatal("whole")
	}
	build := support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	trueBranch, ok := build.Literal(1, true)
	if !ok || !build.Seal() {
		t.Fatal("true branch")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	defer work.Close()
	result, ok := work.Transport(state, trueBranch, forget, whole)
	if !ok || !result.Scope().Same(empty) || sameState(result, state) {
		t.Fatal("forget transport reused source State or scope")
	}
}

func TestMultipleTransportedInputsIntersectAfterTheirOwnBoundaryLaws(t *testing.T) {
	composition, work, state, trueBranch, falseBranch, identity := transportFixture(t)
	defer work.Close()
	whole, ok := support.True(composition.Guards())
	if !ok {
		t.Fatal("whole")
	}
	left, ok := work.Transport(state, trueBranch, identity, whole)
	if !ok {
		t.Fatal("left transport")
	}
	right, ok := work.Transport(state, falseBranch, identity, whole)
	if !ok {
		t.Fatal("right transport")
	}
	intersection, ok := support.Intersect(left.Support(), right.Support())
	if !ok || !intersection.Equal(mustEmpty(t, composition.Guards())) {
		t.Fatal("group inputs did not intersect their transported supports")
	}
}

func mustEmpty(t testing.TB, manager *guard.Manager) support.Mask {
	t.Helper()
	build := support.New(manager)
	if build == nil {
		t.Fatal("support work")
	}
	empty := build.False()
	if !build.Seal() {
		t.Fatal("empty support")
	}
	return empty
}
