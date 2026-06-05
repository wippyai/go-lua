package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReceiverEffectsLatticeLaws(t *testing.T) {
	n := product.FromType(typ.Number)
	s := product.FromType(typ.String)
	optN := product.FromType(typ.NewOptional(typ.Number))

	lattice.LawSuite[ReceiverEffects]{
		Name:   "ReceiverEffects",
		Domain: ReceiverEffectsDomain,
		Sample: []ReceiverEffects{
			ReceiverEffectsDomain.Bottom(),
			ReceiverEffectsDomain.Top(),
			ReceiverEffectsIdentity(),
			ReceiverEffectsOf([]ReceiverEffect{{Slot: 0, Value: n, MustWrite: true}}),
			ReceiverEffectsOf([]ReceiverEffect{{Slot: 0, Value: s, MustWrite: false}}),
			ReceiverEffectsOf([]ReceiverEffect{{Slot: 1, Value: optN, MustWrite: true}}),
			ReceiverEffectsOf([]ReceiverEffect{
				{Slot: 0, Value: n, MustWrite: true},
				{Slot: 2, Value: s, MustWrite: false},
			}),
		},
		Format: func(e ReceiverEffects) string { return e.Format() },
	}.Run(t)
}

func TestReceiverEffectsJoinTurnsMissingPathIntoMayWrite(t *testing.T) {
	write := ReceiverMustWrite(0, product.FromType(typ.String))

	joined := ReceiverEffectsDomain.Join(ReceiverEffectsIdentity(), write)
	entries := joined.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), joined.Format())
	}
	if entries[0].MustWrite {
		t.Fatalf("join(identity, must-write) = must-write, want may-write: %s", joined.Format())
	}
}

func TestReceiverEffectsJoinKeepsMutationOnlyEffect(t *testing.T) {
	mutation := ReceiverMutations(0, []ReceiverMutation{{
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}},
	}})

	joined := ReceiverEffectsDomain.Join(ReceiverEffectsIdentity(), mutation)
	entries := joined.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), joined.Format())
	}
	if entries[0].MustWrite || !entries[0].Value.IsZero() || len(entries[0].Mutations) != 1 {
		t.Fatalf("join(identity, mutation) = %#v, want mutation-only may effect", entries[0])
	}
}

func TestReceiverEffectsKeepsPresentElementMutationDistinctFromBroadMutation(t *testing.T) {
	segments := []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}}
	effects := ReceiverMutations(0, []ReceiverMutation{
		{Segments: segments},
		{Segments: segments, PresentElementWrite: true},
	})

	entries := effects.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), effects.Format())
	}
	mutations := entries[0].Mutations
	if len(mutations) != 2 {
		t.Fatalf("mutations len = %d, want broad and present element: %#v", len(mutations), mutations)
	}
	if mutations[0].PresentElementWrite || !mutations[1].PresentElementWrite {
		t.Fatalf("mutations = %#v, want broad then present element", mutations)
	}
}

func TestReceiverEffectsSequentialComposition(t *testing.T) {
	first := ReceiverMustWrite(0, product.FromType(typ.Number))
	secondMay := ReceiverEffectsOf([]ReceiverEffect{{Slot: 0, Value: product.FromType(typ.String), MustWrite: false}})

	composed := first.Then(secondMay)
	entries := composed.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), composed.Format())
	}
	if !entries[0].MustWrite {
		t.Fatalf("must followed by may must no longer depend on entry value: %s", composed.Format())
	}
	want := product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.String))
	if !product.Domain.Equal(entries[0].Value, want) {
		t.Fatalf("composed value = %v, want %v", entries[0].Value.ProjectValue(), want.ProjectValue())
	}

	third := ReceiverMustWrite(0, product.FromType(typ.Boolean))
	composed = composed.Then(third)
	entries = composed.Entries()
	if len(entries) != 1 || !entries[0].MustWrite || !product.Domain.Equal(entries[0].Value, product.FromType(typ.Boolean)) {
		t.Fatalf("later must-write should override prior effects: %s", composed.Format())
	}
}
