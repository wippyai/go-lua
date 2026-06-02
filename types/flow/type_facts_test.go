package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

type typeFactsStub struct {
	declared  map[cfg.SymbolID]typ.Type
	annotated map[cfg.SymbolID]bool
}

func (s typeFactsStub) DeclaredAt(_ cfg.Point, sym cfg.SymbolID) TypedValue {
	if t := s.declared[sym]; t != nil {
		return TypedValue{Type: t, State: StateResolved}
	}
	return TypedValue{Type: typ.Unknown, State: StateUnknown}
}

func (s typeFactsStub) RefinedAt(_ cfg.Point, _ cfg.SymbolID) TypedValue {
	return TypedValue{Type: nil, State: StateUnknown}
}

func (s typeFactsStub) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) TypedValue {
	return s.DeclaredAt(p, sym)
}

func (s typeFactsStub) IsAnnotated(sym cfg.SymbolID) bool {
	return s.annotated[sym]
}

func TestTypeFacts_Interface(t *testing.T) {
	var _ TypeFacts = (*Solution)(nil)
}

func TestSolution_DeclaredAt_Nil(t *testing.T) {
	var s *Solution
	tv := s.DeclaredAt(0, 0)
	if tv.Type != typ.Unknown {
		t.Error("nil Solution.DeclaredAt should return Unknown")
	}
	if tv.State != StateUnknown {
		t.Error("nil Solution.DeclaredAt should return StateUnknown")
	}
}

func TestSolution_RefinedAt_Nil(t *testing.T) {
	var s *Solution
	tv := s.RefinedAt(0, 0)
	if tv.Type != nil {
		t.Error("nil Solution.RefinedAt should return nil type")
	}
	if tv.State != StateUnknown {
		t.Error("nil Solution.RefinedAt should return StateUnknown")
	}
}

func TestSolution_EffectiveTypeAt_Nil(t *testing.T) {
	var s *Solution
	tv := s.EffectiveTypeAt(0, 0)
	if tv.Type != typ.Unknown {
		t.Error("nil Solution.EffectiveTypeAt should return Unknown")
	}
}

func TestSolution_IsAnnotated_Nil(t *testing.T) {
	var s *Solution
	if s.IsAnnotated(0) {
		t.Error("nil Solution.IsAnnotated should return false")
	}
}

func TestAnnotatedDeclaredPathAtProjectsOnlyExplicitAnnotations(t *testing.T) {
	const sym cfg.SymbolID = 7
	declared := typ.NewRecord().
		Field("box", typ.NewRecord().
			Field("count", typ.Integer).
			Build()).
		Build()
	path := constraint.NewPath(sym, "root").Field("box").Field("count")

	facts := typeFactsStub{
		declared:  map[cfg.SymbolID]typ.Type{sym: declared},
		annotated: map[cfg.SymbolID]bool{sym: true},
	}
	got := AnnotatedDeclaredPathAt(facts, 0, path)
	if got.State != StateResolved || !typ.TypeEquals(got.Type, typ.Integer) {
		t.Fatalf("AnnotatedDeclaredPathAt = %v/%v, want resolved integer", got.State, got.Type)
	}

	unannotated := typeFactsStub{
		declared:  map[cfg.SymbolID]typ.Type{sym: declared},
		annotated: map[cfg.SymbolID]bool{},
	}
	got = AnnotatedDeclaredPathAt(unannotated, 0, path)
	if got.State != StateUnknown {
		t.Fatalf("unannotated path projected as annotation: %v/%v", got.State, got.Type)
	}
}

func TestAnnotatedDeclaredPathSealedUsesProjectedAnnotation(t *testing.T) {
	const sym cfg.SymbolID = 8
	declared := typ.NewRecord().
		Field("sealed", typ.NewRecord().Field("count", typ.Integer).Build()).
		Field("open", typ.NewMap(typ.String, typ.Any)).
		Build()
	facts := typeFactsStub{
		declared:  map[cfg.SymbolID]typ.Type{sym: declared},
		annotated: map[cfg.SymbolID]bool{sym: true},
	}

	sealed := constraint.NewPath(sym, "root").Field("sealed")
	if !AnnotatedDeclaredPathSealed(facts, 0, sealed) {
		t.Fatal("closed record path should be sealed")
	}

	open := constraint.NewPath(sym, "root").Field("open")
	if AnnotatedDeclaredPathSealed(facts, 0, open) {
		t.Fatal("map path should be refinable, not sealed")
	}
}
