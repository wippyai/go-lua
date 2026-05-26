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
