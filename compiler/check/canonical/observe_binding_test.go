package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCanonicalFactsBindingTypesStayOutOfDeclaredFacts(t *testing.T) {
	const point cfg.Point = 3
	const fnSym cfg.SymbolID = 11

	sig := typ.Func().Param("x", typ.Number).Returns(typ.String).Build()
	facts := &canonicalFacts{
		declared: make(map[cfg.SymbolID]typ.Type),
		bindings: map[cfg.SymbolID]typ.Type{fnSym: sig},
	}

	declared := facts.DeclaredAt(point, fnSym)
	if declared.State == flow.StateResolved || !typ.IsUnknown(declared.Type) {
		t.Fatalf("DeclaredAt for binding = %v/%v, want unknown", declared.Type, declared.State)
	}

	effective := facts.EffectiveTypeAt(point, fnSym)
	if effective.State != flow.StateResolved || !typ.TypeEquals(effective.Type, sig) {
		t.Fatalf("EffectiveTypeAt for binding = %v/%v, want %v/resolved", effective.Type, effective.State, sig)
	}
}

func TestCanonicalFactsAnnotatedDeclarationWinsOverBindingType(t *testing.T) {
	const point cfg.Point = 4
	const sym cfg.SymbolID = 12

	sig := typ.Func().Returns(typ.String).Build()
	facts := &canonicalFacts{
		declared: map[cfg.SymbolID]typ.Type{sym: typ.Number},
		annotate: flow.AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{sym: true}),
		bindings: map[cfg.SymbolID]typ.Type{sym: sig},
	}

	effective := facts.EffectiveTypeAt(point, sym)
	if effective.State != flow.StateResolved || !typ.TypeEquals(effective.Type, typ.Number) {
		t.Fatalf("EffectiveTypeAt annotated+binding = %v/%v, want number/resolved", effective.Type, effective.State)
	}
}

func TestBuildObservationInputsKeepsBindingTypesOutOfDeclaredTypes(t *testing.T) {
	const fnSym cfg.SymbolID = 13

	sig := typ.Func().Returns(typ.String).Build()
	inputs := buildObservationInputs(nil, functionFacts{
		declared: map[cfg.SymbolID]typ.Type{},
		bindings: map[cfg.SymbolID]typ.Type{fnSym: sig},
	})

	if inputs.DeclaredTypes[fnSym] != nil {
		t.Fatalf("DeclaredTypes[%d] = %v, want nil", fnSym, inputs.DeclaredTypes[fnSym])
	}
	if !typ.TypeEquals(inputs.BindingTypes[fnSym], sig) {
		t.Fatalf("BindingTypes[%d] = %v, want %v", fnSym, inputs.BindingTypes[fnSym], sig)
	}
}

func TestRecordCallbackEnvBindingTypesKeepsOverlayOutOfDeclaredTypes(t *testing.T) {
	const sym cfg.SymbolID = 14

	overlayType := typ.Func().Returns(typ.Nil).Build()
	facts := functionFacts{
		declared: map[cfg.SymbolID]typ.Type{},
	}

	recordCallbackEnvBindingTypes(&facts, []callbackenv.GlobalBinding{
		{Symbol: sym, Type: overlayType},
	})

	inputs := buildObservationInputs(nil, facts)
	if inputs.DeclaredTypes[sym] != nil {
		t.Fatalf("DeclaredTypes[%d] = %v, want nil", sym, inputs.DeclaredTypes[sym])
	}
	if !typ.TypeEquals(inputs.BindingTypes[sym], overlayType) {
		t.Fatalf("BindingTypes[%d] = %v, want %v", sym, inputs.BindingTypes[sym], overlayType)
	}
}

func TestRecordCallbackEnvBindingTypesDoesNotOverrideDeclaredType(t *testing.T) {
	const sym cfg.SymbolID = 15

	declaredType := typ.Number
	overlayType := typ.Func().Returns(typ.Nil).Build()
	facts := functionFacts{
		declared:  map[cfg.SymbolID]typ.Type{sym: declaredType},
		annotated: flow.AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{sym: true}),
	}

	recordCallbackEnvBindingTypes(&facts, []callbackenv.GlobalBinding{
		{Symbol: sym, Type: overlayType},
	})

	inputs := buildObservationInputs(nil, facts)
	if !typ.TypeEquals(inputs.DeclaredTypes[sym], declaredType) {
		t.Fatalf("DeclaredTypes[%d] = %v, want %v", sym, inputs.DeclaredTypes[sym], declaredType)
	}
	if inputs.BindingTypes[sym] != nil {
		t.Fatalf("BindingTypes[%d] = %v, want nil", sym, inputs.BindingTypes[sym])
	}
}
