package typefacts

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type functionTypeMap map[cfg.SymbolID]typ.Type

func (m functionTypeMap) lookup(sym cfg.SymbolID) typ.Type {
	return m[sym]
}

func TestTypeFactsDeclaredAtPrefersAnnotatedDeclaration(t *testing.T) {
	const sym cfg.SymbolID = 1
	facts := New(Config{
		Declared:      flow.DeclaredTypes{sym: typ.String},
		FunctionType:  functionTypeMap{sym: typ.Number}.lookup,
		Literals:      flow.DeclaredTypes{sym: typ.Boolean},
		AnnotatedVars: map[cfg.SymbolID]bool{sym: true},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("DeclaredAt annotated symbol = %v/%v, want string/resolved", got.Type, got.State)
	}
}

func TestTypeFactsDeclaredAtUsesFunctionBeforeDeclaration(t *testing.T) {
	const sym cfg.SymbolID = 2
	fn := typ.Func().Returns(typ.String).Build()
	facts := New(Config{
		Declared:     flow.DeclaredTypes{sym: typ.Number},
		FunctionType: functionTypeMap{sym: fn}.lookup,
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, fn) {
		t.Fatalf("DeclaredAt function symbol = %v/%v, want canonical function fact", got.Type, got.State)
	}
}

func TestTypeFactsDeclaredAtUsesLiteralLast(t *testing.T) {
	const sym cfg.SymbolID = 3
	facts := New(Config{
		Literals: flow.DeclaredTypes{sym: typ.Boolean},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.Boolean) {
		t.Fatalf("DeclaredAt literal-only symbol = %v/%v, want boolean/resolved", got.Type, got.State)
	}
}

func TestTypeFactsDeclaredAtUnknownIsUnknownState(t *testing.T) {
	const sym cfg.SymbolID = 4
	facts := New(Config{
		Declared: flow.DeclaredTypes{sym: typ.Unknown},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateUnknown || !typ.TypeEquals(got.Type, typ.Unknown) {
		t.Fatalf("DeclaredAt unknown = %v/%v, want unknown/unknown", got.Type, got.State)
	}
}
