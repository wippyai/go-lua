package iteration

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
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
	if gotKind != effect.IterateKeyed || gotIdx != 0 {
		t.Fatalf("Kind() = (%v, %d), want keyed source 0", gotKind, gotIdx)
	}
}

func TestKind_BuiltinFallback(t *testing.T) {
	tests := []struct {
		name     string
		builtin  string
		wantKind effect.IteratorKind
		wantOK   bool
	}{
		{name: "ipairs", builtin: "ipairs", wantKind: effect.IterateIndexed, wantOK: true},
		{name: "pairs", builtin: "pairs", wantKind: effect.IterateKeyed, wantOK: true},
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

func TestVarTypes_IndexedArrayAndDynamicSource(t *testing.T) {
	got, ok := VarTypes(effect.IterateIndexed, 3, typ.NewArray(typ.String))
	if !ok {
		t.Fatal("VarTypes indexed array did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.Integer) || !typ.TypeEquals(got[1], typ.String) || got[2] != nil {
		t.Fatalf("indexed array vars = %#v, want integer/string/nil", got)
	}

	got, ok = VarTypes(effect.IterateIndexed, 2, typ.NewOptional(typ.Any))
	if !ok {
		t.Fatal("VarTypes indexed optional-any did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.Integer) || !typ.TypeEquals(got[1], typ.Any) {
		t.Fatalf("indexed optional-any vars = %#v, want integer/any", got)
	}
}

func TestVarTypes_KeyedUniformContainers(t *testing.T) {
	mapType := typ.NewMap(typ.String, typ.Number)
	got, ok := VarTypes(effect.IterateKeyed, 2, mapType)
	if !ok {
		t.Fatal("VarTypes keyed map did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.String) || !typ.TypeEquals(got[1], typ.Number) {
		t.Fatalf("keyed map vars = %#v, want string/number", got)
	}

	recordMap := typ.NewRecord().
		Field("fixed", typ.Boolean).
		MapComponent(typ.String, typ.Number).
		Build()
	got, ok = VarTypes(effect.IterateKeyed, 2, recordMap)
	if !ok {
		t.Fatal("VarTypes keyed record map did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.String) || !typ.TypeEquals(got[1], typ.NewUnion(typ.Boolean, typ.Number)) {
		t.Fatalf("keyed record-map vars = %#v, want string/boolean|number", got)
	}
}

func TestVarTypes_KeyedAnyAndClosedRecord(t *testing.T) {
	got, ok := VarTypes(effect.IterateKeyed, 2, typ.Any)
	if !ok {
		t.Fatal("VarTypes keyed any did not resolve")
	}
	if !typ.TypeEquals(got[0], typ.Any) || !typ.TypeEquals(got[1], typ.Any) {
		t.Fatalf("keyed any vars = %#v, want any/any", got)
	}

	closed := typ.NewRecord().Field("name", typ.String).Build()
	got, ok = VarTypes(effect.IterateKeyed, 2, closed)
	if !ok {
		t.Fatal("VarTypes rejected closed record with finite present entries")
	}
	if !typ.TypeEquals(got[0], typ.LiteralString("name")) || !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("closed record vars = %#v, want literal name/string", got)
	}

	if _, ok := VarTypes(effect.IterateKeyed, 2, typ.NewMap(typ.String, typ.Nil)); ok {
		t.Fatal("VarTypes accepted keyed map with no present entries")
	}
}

func TestProjectVarTypes_KeyedEmptyContainer(t *testing.T) {
	proj, ok := ProjectVarTypes(effect.IterateKeyed, 2, typ.NewRecord().Build())
	if !ok || !proj.Empty {
		t.Fatalf("ProjectVarTypes(empty record) = %#v, %v; want recognized empty", proj, ok)
	}
	proj, ok = ProjectVarTypes(effect.IterateKeyed, 2, typ.NewMap(typ.String, typ.Nil))
	if !ok || !proj.Empty {
		t.Fatalf("ProjectVarTypes(nil-valued map) = %#v, %v; want recognized empty", proj, ok)
	}
	proj, ok = ProjectVarTypes(effect.IterateKeyed, 2, typ.Never)
	if !ok || !proj.Empty {
		t.Fatalf("ProjectVarTypes(never) = %#v, %v; want recognized empty", proj, ok)
	}
	proj, ok = ProjectVarTypes(effect.IterateIndexed, 2, typ.Never)
	if !ok || !proj.Empty {
		t.Fatalf("ProjectVarTypes(indexed never) = %#v, %v; want recognized empty", proj, ok)
	}
}

func TestIsUniformKeyedContainer(t *testing.T) {
	if !IsUniformKeyedContainer(typ.NewMap(typ.String, typ.Number)) {
		t.Fatal("map should be uniform keyed")
	}
	if !IsUniformKeyedContainer(typ.NewRecord().Field("kind", typ.LiteralString("x")).Build()) {
		t.Fatal("closed field-only record should be a finite keyed iterator")
	}
	open := typ.NewRecord().Field("kind", typ.String).SetOpen(true).Build()
	if !IsUniformKeyedContainer(open) {
		t.Fatal("open record should be uniform keyed")
	}
	if IsUniformKeyedContainer(typ.String) || IsUniformKeyedContainer(nil) {
		t.Fatal("non-container should not be uniform keyed")
	}
}

func TestVarTypes_RejectsUnknownAndInvalidKind(t *testing.T) {
	if _, ok := VarTypes(effect.IterateIndexed, 2, typ.Unknown); ok {
		t.Fatal("unknown source should not project iterator vars")
	}
	if _, ok := VarTypes(effect.IteratorKind(99), 2, typ.NewArray(typ.String)); ok {
		t.Fatal("invalid iterator kind should not project vars")
	}
	if _, ok := VarTypes(effect.IterateIndexed, 0, typ.NewArray(typ.String)); ok {
		t.Fatal("zero target count should not project vars")
	}
	if got, ok := VarTypes(effect.IterateIndexed, 2, typ.Nil); ok && got[1] != nil && got[1].Kind() != kind.Nil {
		t.Fatalf("nil source should not invent non-nil iterator element: %#v", got)
	}
}
