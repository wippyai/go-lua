package iteration

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestKind_ContractIteratorWinsOverBuiltinName(t *testing.T) {
	fn := typ.Func().
		Param("items", typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   effect.IterateKeyed,
		})).
		Build()

	gotKind, gotIdx, ok := Kind(fn, "ipairs", 1)
	if !ok {
		t.Fatal("Kind() did not resolve contract iterator")
	}
	if gotKind != flow.IterateKeyed || gotIdx != 0 {
		t.Fatalf("Kind() = (%v, %d), want keyed source 0", gotKind, gotIdx)
	}
}

func TestKind_BuiltinFallback(t *testing.T) {
	tests := []struct {
		name     string
		builtin  string
		wantKind flow.IteratorKind
		wantOK   bool
	}{
		{name: "ipairs", builtin: "ipairs", wantKind: flow.IterateIndexed, wantOK: true},
		{name: "pairs", builtin: "pairs", wantKind: flow.IterateKeyed, wantOK: true},
		{name: "other", builtin: "next", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotIdx, ok := Kind(nil, tt.builtin, 1)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if gotKind != tt.wantKind || gotIdx != 0 {
				t.Fatalf("Kind() = (%v, %d), want (%v, 0)", gotKind, gotIdx, tt.wantKind)
			}
		})
	}
}

func TestBuiltinName_RequiresBoundGlobalSymbol(t *testing.T) {
	bindings := bind.NewBindingTable()

	globalPairs := ident("pairs")
	bindIdent(bindings, globalPairs, 1)
	bindings.SetKind(1, cfg.SymbolGlobal)
	if got := BuiltinName(globalPairs, bindings); got != "pairs" {
		t.Fatalf("BuiltinName(global pairs) = %q, want pairs", got)
	}

	globalIPairs := ident("ipairs")
	bindIdent(bindings, globalIPairs, 2)
	bindings.SetKind(2, cfg.SymbolGlobal)
	if got := BuiltinName(globalIPairs, bindings); got != "ipairs" {
		t.Fatalf("BuiltinName(global ipairs) = %q, want ipairs", got)
	}

	localPairs := ident("pairs")
	bindIdent(bindings, localPairs, 3)
	bindings.SetKind(3, cfg.SymbolLocal)
	if got := BuiltinName(localPairs, bindings); got != "" {
		t.Fatalf("BuiltinName(local pairs) = %q, want empty", got)
	}

	paramIPairs := ident("ipairs")
	bindIdent(bindings, paramIPairs, 4)
	bindings.SetKind(4, cfg.SymbolParam)
	if got := BuiltinName(paramIPairs, bindings); got != "" {
		t.Fatalf("BuiltinName(param ipairs) = %q, want empty", got)
	}

	globalNext := ident("next")
	bindIdent(bindings, globalNext, 5)
	bindings.SetKind(5, cfg.SymbolGlobal)
	if got := BuiltinName(globalNext, bindings); got != "" {
		t.Fatalf("BuiltinName(global next) = %q, want empty", got)
	}

	mismatch := ident("pairs")
	bindings.Bind(mismatch, 6)
	bindings.SetName(6, "other")
	bindings.SetKind(6, cfg.SymbolGlobal)
	if got := BuiltinName(mismatch, bindings); got != "" {
		t.Fatalf("BuiltinName(name mismatch) = %q, want empty", got)
	}

	if got := BuiltinName(ident("pairs"), bindings); got != "" {
		t.Fatalf("BuiltinName(unbound pairs) = %q, want empty", got)
	}
	if got := BuiltinName(&ast.StringExpr{Value: "pairs"}, bindings); got != "" {
		t.Fatalf("BuiltinName(non-ident) = %q, want empty", got)
	}
	if got := BuiltinName(globalPairs, nil); got != "" {
		t.Fatalf("BuiltinName(nil bindings) = %q, want empty", got)
	}
}

func TestKind_RejectsOutOfRangeContractSource(t *testing.T) {
	fn := typ.Func().
		Param("items", typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Iterator{
			Source: effect.ParamRef{Index: 2},
			Kind:   effect.IterateIndexed,
		})).
		Build()

	if _, _, ok := Kind(fn, "", 1); ok {
		t.Fatal("Kind() accepted out-of-range iterator source")
	}
}
