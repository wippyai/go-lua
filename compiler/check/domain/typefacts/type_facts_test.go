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
		AnnotatedVars: flow.AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{sym: true}),
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("DeclaredAt annotated symbol = %v/%v, want string/resolved", got.Type, got.State)
	}
}

func TestTypeFactsSeparatesBindingValueFromDeclaration(t *testing.T) {
	const sym cfg.SymbolID = 2
	fn := typ.Func().Returns(typ.String).Build()
	facts := New(Config{
		Declared:     flow.DeclaredTypes{sym: typ.Number},
		FunctionType: functionTypeMap{sym: fn}.lookup,
	})

	declared := facts.DeclaredAt(0, sym)
	if declared.State != flow.StateResolved || !typ.TypeEquals(declared.Type, typ.Number) {
		t.Fatalf("DeclaredAt function symbol = %v/%v, want declared number/resolved", declared.Type, declared.State)
	}

	binding := facts.BindingValueAt(0, sym)
	if binding.State != flow.StateResolved || !typ.TypeEquals(binding.Type, fn) {
		t.Fatalf("BindingValueAt function symbol = %v/%v, want canonical function fact", binding.Type, binding.State)
	}

	effective := facts.EffectiveTypeAt(0, sym)
	if effective.State != flow.StateResolved || !typ.TypeEquals(effective.Type, fn) {
		t.Fatalf("EffectiveTypeAt function symbol = %v/%v, want canonical function fact", effective.Type, effective.State)
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

func TestSelectEffectiveKeepsKnownDeclarationOverRefinedUnknown(t *testing.T) {
	declared := typ.NewMap(typ.String, typ.Any)

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: typ.Unknown, State: flow.StateResolved},
		false,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, declared) {
		t.Fatalf("SelectEffective refined unknown = %v/%v, want declared %v/resolved", got.Type, got.State, declared)
	}
}

func TestSelectEffectiveAnnotatedAnyAdoptsProvenRefinement(t *testing.T) {
	refined := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()

	got := SelectEffective(
		flow.TypedValue{Type: typ.Any, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, refined) {
		t.Fatalf("SelectEffective annotated any refinement = %v/%v, want %v/resolved", got.Type, got.State, refined)
	}
}

func TestSelectEffectiveAnnotatedConcreteAdoptsSubtypeRefinement(t *testing.T) {
	refined := typ.LiteralString("ready")

	got := SelectEffective(
		flow.TypedValue{Type: typ.String, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, refined) {
		t.Fatalf("SelectEffective annotated string refinement = %v/%v, want %v/resolved", got.Type, got.State, refined)
	}
}

func TestSelectEffectiveAnnotatedSoftContainerAdoptsPrecisionRefinement(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	declared := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	refined := typ.NewRecursive("Flow", func(self typ.Type) typ.Type {
		return typ.NewMap(typ.String, typ.NewArray(entry))
	})

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	want := typ.NewMap(typ.String, typ.NewArray(entry))
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, want) {
		t.Fatalf("SelectEffective annotated soft container = %v/%v, want %v/resolved", got.Type, got.State, want)
	}
}

func TestSelectEffectiveAnnotatedMapKeepsMapComponentWhenFlowObservationIsRecordFields(t *testing.T) {
	declared := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	refined := typ.NewRecord().
		Field("a", typ.NewTuple(typ.LiteralInt(1))).
		Field("b", typ.NewTuple(typ.LiteralInt(2))).
		SetOpen(true).
		Build()

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	want := typ.NewMap(typ.String, typ.NewUnion(typ.NewTuple(typ.LiteralInt(1)), typ.NewTuple(typ.LiteralInt(2))))
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, want) {
		t.Fatalf("SelectEffective annotated map with field observation = %v/%v, want %v/resolved", got.Type, got.State, want)
	}
}

func TestSelectEffectiveAnnotatedRecordUsesDeclaredContractOverInitWitnessUnion(t *testing.T) {
	declared := typ.NewRecord().
		Field("run_with", typ.Func().Param("self", typ.Any).Param("db", typ.String).Returns(typ.Any).Build()).
		Build()
	refined := typ.NewUnion(declared, typ.NewRecord().Build())

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, declared) {
		t.Fatalf("SelectEffective annotated record witness union = %v/%v, want declared %v/resolved", got.Type, got.State, declared)
	}
}
