package store

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewInterprocState(t *testing.T) {
	state := NewInterprocState()
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Facts == nil {
		t.Error("Facts map should be initialized")
	}
	if state.Effects == nil {
		t.Error("Effects map should be initialized")
	}
	if state.ConstructorFields == nil {
		t.Error("ConstructorFields map should be initialized")
	}
}

func TestEffectsEqual_BothNil(t *testing.T) {
	if !effectsEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
}

func TestEffectsEqual_OneNil(t *testing.T) {
	eff := &constraint.FunctionEffect{}
	if effectsEqual(eff, nil) {
		t.Error("non-nil and nil should not be equal")
	}
	if effectsEqual(nil, eff) {
		t.Error("nil and non-nil should not be equal")
	}
}

func TestEffectsEqual_Same(t *testing.T) {
	eff := &constraint.FunctionEffect{Terminates: true}
	if !effectsEqual(eff, eff) {
		t.Error("same reference should be equal")
	}
}

func TestEffectsMapEqual_Empty(t *testing.T) {
	if !effectsMapEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
	if !effectsMapEqual(map[cfg.SymbolID]*constraint.FunctionEffect{}, map[cfg.SymbolID]*constraint.FunctionEffect{}) {
		t.Error("two empty maps should be equal")
	}
}

func TestEffectsMapEqual_DifferentLength(t *testing.T) {
	a := map[cfg.SymbolID]*constraint.FunctionEffect{1: {}}
	b := map[cfg.SymbolID]*constraint.FunctionEffect{}
	if effectsMapEqual(a, b) {
		t.Error("maps of different length should not be equal")
	}
}

func TestInterprocFactsMapEqual_Empty(t *testing.T) {
	if !interprocFactsMapEqual(nil, nil) {
		t.Error("two nils should be equal")
	}
	if !interprocFactsMapEqual(map[api.GraphKey]api.Facts{}, map[api.GraphKey]api.Facts{}) {
		t.Error("two empty maps should be equal")
	}
}

func TestInterprocFactsMapEqual_DifferentLength(t *testing.T) {
	a := map[api.GraphKey]api.Facts{{GraphID: 1}: {}}
	b := map[api.GraphKey]api.Facts{}
	if interprocFactsMapEqual(a, b) {
		t.Error("maps of different length should not be equal")
	}
}

func TestWidenInterprocFacts_Empty(t *testing.T) {
	result := widenInterprocFacts(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Error("expected empty map")
	}
}

func TestWidenInterprocFacts_OnlyPrev(t *testing.T) {
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {ReturnSummaries: map[cfg.SymbolID][]typ.Type{1: {typ.String}}},
	}
	result := widenInterprocFacts(prev, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_OnlyNext(t *testing.T) {
	next := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {ReturnSummaries: map[cfg.SymbolID][]typ.Type{1: {typ.Number}}},
	}
	result := widenInterprocFacts(nil, next)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestWidenInterprocFacts_Merge(t *testing.T) {
	prev := map[api.GraphKey]api.Facts{
		{GraphID: 1}: {ReturnSummaries: map[cfg.SymbolID][]typ.Type{1: {typ.String}}},
	}
	next := map[api.GraphKey]api.Facts{
		{GraphID: 2}: {ReturnSummaries: map[cfg.SymbolID][]typ.Type{1: {typ.Number}}},
	}
	result := widenInterprocFacts(prev, next)
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
}

func TestSessionStore_Fields(t *testing.T) {
	s := &SessionStore{
		Module: &ModuleStore{
			Graphs: make(map[uint64]*cfg.Graph),
		},
		Iteration: &IterationStore{
			Revision: 5,
		},
	}
	if s.Module == nil {
		t.Error("Module should be set")
	}
	if s.Iteration.Revision != 5 {
		t.Error("Revision should be 5")
	}
}

func TestModuleStore_Fields(t *testing.T) {
	m := &ModuleStore{
		Graphs:        make(map[uint64]*cfg.Graph),
		Parents:       make(map[uint64]*scope.State),
		ModuleAliases: map[cfg.SymbolID]string{1: "test"},
	}
	if m.ModuleAliases[1] != "test" {
		t.Error("ModuleAliases not set correctly")
	}
}

func TestFunctionRegistry_Fields(t *testing.T) {
	r := &FunctionRegistry{
		BySym:     make(map[cfg.SymbolID]*api.FunctionRef),
		ByFunc:    make(map[*ast.FunctionExpr]*api.FunctionRef),
		ByGraphID: make(map[uint64]*api.FunctionRef),
	}
	if r.BySym == nil {
		t.Error("BySym should be initialized")
	}
}

func TestIterationStore_Fields(t *testing.T) {
	i := &IterationStore{Revision: 10}
	if i.Revision != 10 {
		t.Error("Revision not set")
	}
}

func TestIterationScratch_Fields(t *testing.T) {
	s := &IterationScratch{
		LiteralSigsByGraphID: make(map[uint64]map[*ast.FunctionExpr]*typ.Function),
	}
	if s.LiteralSigsByGraphID == nil {
		t.Error("LiteralSigsByGraphID should be initialized")
	}
}
