package query

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type functionFactMap map[cfg.SymbolID]typ.Type

func (m functionFactMap) FunctionType(sym cfg.SymbolID) typ.Type {
	return m[sym]
}

func TestTypeFactsDeclaredAtPrefersAnnotatedDeclaration(t *testing.T) {
	const sym cfg.SymbolID = 1
	facts := NewTypeFacts(TypeFactsConfig{
		Declared:      flow.DeclaredTypes{sym: typ.String},
		Functions:     functionFactMap{sym: typ.Number},
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
	facts := NewTypeFacts(TypeFactsConfig{
		Declared:  flow.DeclaredTypes{sym: typ.Number},
		Functions: functionFactMap{sym: fn},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, fn) {
		t.Fatalf("DeclaredAt function symbol = %v/%v, want canonical function fact", got.Type, got.State)
	}
}

func TestTypeFactsDeclaredAtUsesLiteralLast(t *testing.T) {
	const sym cfg.SymbolID = 3
	facts := NewTypeFacts(TypeFactsConfig{
		Literals: flow.DeclaredTypes{sym: typ.Boolean},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.Boolean) {
		t.Fatalf("DeclaredAt literal-only symbol = %v/%v, want boolean/resolved", got.Type, got.State)
	}
}

func TestTypeFactsDeclaredAtUnknownIsUnknownState(t *testing.T) {
	const sym cfg.SymbolID = 4
	facts := NewTypeFacts(TypeFactsConfig{
		Declared: flow.DeclaredTypes{sym: typ.Unknown},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateUnknown || !typ.TypeEquals(got.Type, typ.Unknown) {
		t.Fatalf("DeclaredAt unknown = %v/%v, want unknown/unknown", got.Type, got.State)
	}
}
