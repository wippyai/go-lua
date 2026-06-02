package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestAggregateCellEffects_DirectCalleesUseEntryValuesAndCooccur(t *testing.T) {
	refA := summary.FuncRef{GraphID: 1}
	refB := summary.FuncRef{GraphID: 2}
	slotValue := product.FromType(typ.Boolean)
	effectA := flow.CaptureMustWrite(cfg.SymbolID(10), product.FromType(typ.String))
	effectB := flow.CaptureMustWrite(cfg.SymbolID(11), product.FromType(typ.Number))

	got := summary.AggregateCellEffects(summary.CellEffectAggregation{
		DirectRefs: []summary.FuncRef{refA, refB},
		DirectEntryValues: func(ref summary.FuncRef) summary.EntryValues {
			if ref != refA {
				return nil
			}
			return summary.EntryValues{0: slotValue}
		},
		EffectOf: func(ref summary.FuncRef, entry summary.EntryValues) flow.CaptureEffects {
			switch ref {
			case refA:
				if len(entry) != 1 || !product.Equal(entry[0], slotValue) {
					t.Fatalf("entry values for refA = %#v, want slot 0", entry)
				}
				return effectA
			case refB:
				if len(entry) != 0 {
					t.Fatalf("entry values for refB = %#v, want none", entry)
				}
				return effectB
			default:
				t.Fatalf("unexpected ref %v", ref)
				return flow.CaptureEffectsDomain.Bottom()
			}
		},
	})

	want := flow.CooccurringCaptureEffects(effectA, effectB)
	if !flow.CaptureEffectsDomain.Equal(got, want) {
		t.Fatalf("effects = %s, want %s", got.Format(), want.Format())
	}
}

func TestAggregateCellEffects_CallbacksAreSortedMethodIndexedAndWeakened(t *testing.T) {
	arg0 := &ast.IdentExpr{Value: "first"}
	arg1 := &ast.IdentExpr{Value: "second"}
	refMay := summary.FuncRef{GraphID: 10}
	refMust := summary.FuncRef{GraphID: 11}
	effectMayBase := flow.CaptureMustWrite(cfg.SymbolID(20), product.FromType(typ.String))
	effectMust := flow.CaptureMustWrite(cfg.SymbolID(21), product.FromType(typ.Number))
	var order []summary.FuncRef

	got := summary.AggregateCellEffects(summary.CellEffectAggregation{
		CallbackSpec: contract.NewSpec().
			WithCallback(2, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}).
			WithCallback(1, &contract.CallbackSpec{Cardinality: contract.CardAtMostOnce}),
		CallbackArgs: []ast.Expr{arg0, arg1},
		MethodCall:   true,
		ResolveCallback: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			switch arg {
			case arg0:
				return []summary.FuncRef{refMay}, true
			case arg1:
				return []summary.FuncRef{refMust}, true
			default:
				return nil, false
			}
		},
		EffectOf: func(ref summary.FuncRef, entry summary.EntryValues) flow.CaptureEffects {
			if len(entry) != 0 {
				t.Fatalf("callback entry values = %#v, want none", entry)
			}
			order = append(order, ref)
			switch ref {
			case refMay:
				return effectMayBase
			case refMust:
				return effectMust
			default:
				t.Fatalf("unexpected callback ref %v", ref)
				return flow.CaptureEffectsDomain.Bottom()
			}
		},
	})

	if len(order) != 2 || order[0] != refMay || order[1] != refMust {
		t.Fatalf("callback order = %#v, want sorted by parameter index", order)
	}
	want := flow.CooccurringCaptureEffects(effectMayBase.May(), effectMust)
	if !flow.CaptureEffectsDomain.Equal(got, want) {
		t.Fatalf("effects = %s, want %s", got.Format(), want.Format())
	}
	entries := got.Entries()
	for _, entry := range entries {
		if entry.Symbol == cfg.SymbolID(20) && entry.MustWrite {
			t.Fatalf("at-most-once callback was not weakened: %s", got.Format())
		}
	}
}

func TestAggregateCellEffects_DirectEffectsCanBeSuppliedPreFolded(t *testing.T) {
	direct := flow.CooccurringCaptureEffects(
		flow.CaptureMustWrite(cfg.SymbolID(10), product.FromType(typ.String)),
		flow.CaptureMustWrite(cfg.SymbolID(11), product.FromType(typ.Number)),
	)

	callbackRef := summary.FuncRef{GraphID: 20}
	callbackEffect := flow.CaptureMustWrite(cfg.SymbolID(12), product.FromType(typ.Boolean))
	arg := &ast.IdentExpr{Value: "cb"}

	got := summary.AggregateCellEffects(summary.CellEffectAggregation{
		DirectEffects: direct,
		CallbackSpec:  contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}),
		CallbackArgs:  []ast.Expr{arg},
		MethodCall:    false,
		ResolveCallback: func(expr ast.Expr) ([]summary.FuncRef, bool) {
			if expr == arg {
				return []summary.FuncRef{callbackRef}, true
			}
			return nil, false
		},
		EffectOf: func(ref summary.FuncRef, entry summary.EntryValues) flow.CaptureEffects {
			if len(entry) != 0 {
				t.Fatalf("callback entry values = %#v, want none", entry)
			}
			if ref != callbackRef {
				t.Fatalf("unexpected callback ref %v", ref)
				return flow.CaptureEffectsDomain.Bottom()
			}
			return callbackEffect
		},
	})

	want := flow.CooccurringCaptureEffects(direct, callbackEffect)
	if !flow.CaptureEffectsDomain.Equal(got, want) {
		t.Fatalf("effects = %s, want %s", got.Format(), want.Format())
	}
}

func TestAggregateCellEffects_NoLookupIsBottom(t *testing.T) {
	got := summary.AggregateCellEffects(summary.CellEffectAggregation{
		DirectRefs: []summary.FuncRef{{GraphID: 1}},
	})
	if !flow.CaptureEffectsDomain.Equal(got, flow.CaptureEffectsDomain.Bottom()) {
		t.Fatalf("effects = %s, want bottom", got.Format())
	}
}

func TestAggregateCellEffects_FoldsEachFiniteCallbackTarget(t *testing.T) {
	arg := &ast.IdentExpr{Value: "cb"}
	refA := summary.FuncRef{GraphID: 20}
	refB := summary.FuncRef{GraphID: 21}
	effectA := flow.CaptureMustWrite(cfg.SymbolID(30), product.FromType(typ.String))
	effectB := flow.CaptureMustWrite(cfg.SymbolID(31), product.FromType(typ.Number))

	got := summary.AggregateCellEffects(summary.CellEffectAggregation{
		CallbackSpec: contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}),
		CallbackArgs: []ast.Expr{arg},
		ResolveCallback: func(expr ast.Expr) ([]summary.FuncRef, bool) {
			if expr != arg {
				return nil, false
			}
			return []summary.FuncRef{refA, refB}, true
		},
		EffectOf: func(ref summary.FuncRef, _ summary.EntryValues) flow.CaptureEffects {
			switch ref {
			case refA:
				return effectA
			case refB:
				return effectB
			default:
				t.Fatalf("unexpected callback ref %v", ref)
				return flow.CaptureEffectsDomain.Bottom()
			}
		},
	})

	want := flow.CooccurringCaptureEffects(effectA, effectB)
	if !flow.CaptureEffectsDomain.Equal(got, want) {
		t.Fatalf("effects = %s, want %s", got.Format(), want.Format())
	}
}
