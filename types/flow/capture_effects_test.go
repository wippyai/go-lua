package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCaptureEffectsLatticeLaws(t *testing.T) {
	n := product.FromType(typ.Number)
	s := product.FromType(typ.String)
	optN := product.FromType(typ.NewOptional(typ.Number))

	lattice.LawSuite[CaptureEffects]{
		Name:   "CaptureEffects",
		Domain: CaptureEffectsDomain,
		Sample: []CaptureEffects{
			CaptureEffectsDomain.Bottom(),
			CaptureEffectsDomain.Top(),
			CaptureEffectsIdentity(),
			CaptureEffectsOf([]CaptureEffect{{Symbol: 1, Value: n, MustWrite: true}}),
			CaptureEffectsOf([]CaptureEffect{{Symbol: 1, Value: s, MustWrite: false}}),
			CaptureEffectsOf([]CaptureEffect{{Symbol: 2, Value: optN, MustWrite: true}}),
			CaptureEffectsOf([]CaptureEffect{
				{Symbol: 1, Value: n, MustWrite: true},
				{Symbol: 2, Value: s, MustWrite: false},
			}),
		},
		Format: func(e CaptureEffects) string { return e.Format() },
	}.Run(t)
}

func TestCaptureEffectsJoinTurnsMissingPathIntoMayWrite(t *testing.T) {
	write := CaptureMustWrite(1, product.FromType(typ.String))

	joined := CaptureEffectsDomain.Join(CaptureEffectsIdentity(), write)
	entries := joined.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), joined.Format())
	}
	if entries[0].MustWrite {
		t.Fatalf("join(identity, must-write) = must-write, want may-write: %s", joined.Format())
	}
}

func TestCaptureEffectsApplyMustAndMay(t *testing.T) {
	store := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(1), Value: product.FromType(typ.Number)}})

	must := CaptureMustWrite(1, product.FromType(typ.String))
	got := must.Apply(store)
	if v, ok := got.Value(1); !ok || !product.Domain.Equal(v, product.FromType(typ.String)) {
		t.Fatalf("must apply cell = %v/%v, want string", v.ProjectValue(), ok)
	}

	may := CaptureEffectsOf([]CaptureEffect{{Symbol: 1, Value: product.FromType(typ.Boolean), MustWrite: false}})
	got = may.Apply(store)
	want := product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.Boolean))
	if v, ok := got.Value(1); !ok || !product.Domain.Equal(v, want) {
		t.Fatalf("may apply cell = %v/%v, want %v", v.ProjectValue(), ok, want.ProjectValue())
	}
}

func TestCaptureEffectsMayWeakensMustWrites(t *testing.T) {
	must := CaptureMustWrite(1, product.FromType(typ.String))

	got := must.May()
	entries := got.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), got.Format())
	}
	if entries[0].MustWrite {
		t.Fatalf("May() kept must-write: %s", got.Format())
	}

	store := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(1), Value: product.FromType(typ.Number)}})
	applied := got.Apply(store)
	want := product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.String))
	if v, ok := applied.Value(1); !ok || !product.Domain.Equal(v, want) {
		t.Fatalf("may apply cell = %v/%v, want %v", v.ProjectValue(), ok, want.ProjectValue())
	}
}

func TestCaptureEffectsSequentialComposition(t *testing.T) {
	first := CaptureMustWrite(1, product.FromType(typ.Number))
	secondMay := CaptureEffectsOf([]CaptureEffect{{Symbol: 1, Value: product.FromType(typ.String), MustWrite: false}})

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

	third := CaptureMustWrite(1, product.FromType(typ.Boolean))
	composed = composed.Then(third)
	entries = composed.Entries()
	if len(entries) != 1 || !entries[0].MustWrite || !product.Domain.Equal(entries[0].Value, product.FromType(typ.Boolean)) {
		t.Fatalf("later must-write should override prior effects: %s", composed.Format())
	}
}

func TestCooccurringCaptureEffectsUnknownOrder(t *testing.T) {
	writeString := CaptureMustWrite(1, product.FromType(typ.String))
	writeNumber := CaptureMustWrite(1, product.FromType(typ.Number))

	got := CooccurringCaptureEffects(writeString, writeNumber)
	if !CaptureEffectsDomain.Equal(got, CooccurringCaptureEffects(writeNumber, writeString)) {
		t.Fatalf("cooccurring effects are not commutative: %s", got.Format())
	}
	entries := got.Entries()
	if len(entries) != 1 || !entries[0].MustWrite {
		t.Fatalf("cooccurring must writes = %s, want one must write", got.Format())
	}
	want := product.Domain.Join(product.FromType(typ.String), product.FromType(typ.Number))
	if !product.Domain.Equal(entries[0].Value, want) {
		t.Fatalf("cooccurring value = %v, want %v", entries[0].Value.ProjectValue(), want.ProjectValue())
	}
}

func TestCooccurringCaptureEffectsBottomIdentity(t *testing.T) {
	write := CaptureMustWrite(1, product.FromType(typ.String))

	if got := CooccurringCaptureEffects(CaptureEffectsDomain.Bottom(), write); !CaptureEffectsDomain.Equal(got, write) {
		t.Fatalf("bottom cooccurring write = %s, want %s", got.Format(), write.Format())
	}
	if got := CooccurringCaptureEffects(write, CaptureEffectsDomain.Bottom()); !CaptureEffectsDomain.Equal(got, write) {
		t.Fatalf("write cooccurring bottom = %s, want %s", got.Format(), write.Format())
	}
}
