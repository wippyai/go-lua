package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInputsEqual_RecursiveProductsUseFactEquality(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("suite")).
			Field("children", typ.NewArray(self)).
			Build()
	})
	different := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("case")).
			Field("children", typ.NewArray(self)).
			Build()
	})

	a := &Inputs{DeclaredTypes: map[cfg.SymbolID]typ.Type{1: left}}
	b := &Inputs{DeclaredTypes: map[cfg.SymbolID]typ.Type{1: right}}
	if !InputsEqual(a, b) {
		t.Fatal("equivalent recursive flow input products should compare equal")
	}

	b.DeclaredTypes[1] = different
	if InputsEqual(a, b) {
		t.Fatal("different recursive flow input products should not compare equal")
	}
}

func TestInputsEqual_BindingTypesAreSemanticInputs(t *testing.T) {
	sym := cfg.SymbolID(7)
	stringFn := typ.Func().Returns(typ.String).Build()
	numberFn := typ.Func().Returns(typ.Number).Build()

	a := &Inputs{BindingTypes: map[cfg.SymbolID]typ.Type{sym: stringFn}}
	b := &Inputs{BindingTypes: map[cfg.SymbolID]typ.Type{sym: typ.Func().Returns(typ.String).Build()}}
	if !InputsEqual(a, b) {
		t.Fatal("equivalent binding type overlays should compare equal")
	}

	b.BindingTypes[sym] = numberFn
	if InputsEqual(a, b) {
		t.Fatal("different binding type overlays should not compare equal")
	}
}
