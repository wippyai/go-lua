package effects

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPropagate_NilResult(t *testing.T) {
	result := Propagate(nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil result, got %v", result)
	}
}

func TestPropagate_EmptyResult(t *testing.T) {
	result := Propagate(&api.FuncResult{}, nil)
	if result == nil {
		t.Fatal("expected non-nil effect for empty result")
	}
}

func TestPropagate_WithLocalEffect(t *testing.T) {
	fnEffect := &constraint.FunctionEffect{
		Terminates: true,
	}
	result := Propagate(&api.FuncResult{FnEffect: fnEffect}, nil)
	if result == nil {
		t.Fatal("expected non-nil effect")
	}
	if !result.Terminates {
		t.Error("expected Terminates to be true")
	}
}

func TestLookupEffectBySym_NilStore(t *testing.T) {
	result := LookupEffectBySym(nil, nil, nil, 1)
	if result != nil {
		t.Errorf("expected nil for nil store, got %v", result)
	}
}

func TestLookupEffectBySym_ZeroSym(t *testing.T) {
	result := LookupEffectBySym(nil, nil, nil, 0)
	if result != nil {
		t.Errorf("expected nil for zero symbol, got %v", result)
	}
}

func TestTerminatesFromReachability_NilResult(t *testing.T) {
	if TerminatesFromReachability(nil) {
		t.Error("expected false for nil result")
	}
}

func TestTerminatesFromReachability_NilFlowSolution(t *testing.T) {
	if TerminatesFromReachability(&api.FuncResult{}) {
		t.Error("expected false for nil flow solution")
	}
}

func TestEffectFromType_Nil(t *testing.T) {
	if EffectFromType(nil) != nil {
		t.Error("expected nil for nil type")
	}
}

func TestEffectFromType_NonFunction(t *testing.T) {
	if EffectFromType(typ.String) != nil {
		t.Error("expected nil for non-function type")
	}
}

func TestEffectFromType_PureFunction(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	result := EffectFromType(fn)
	if result != nil {
		t.Errorf("expected nil for pure function, got %v", result)
	}
}

func TestEffectFromType_WithEffects(t *testing.T) {
	row := effect.Empty.With(effect.IO{})
	fn := typ.Func().Returns(typ.String).Effects(row).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect for function with effects")
	}
}

func TestEffectFromType_NeverReturn(t *testing.T) {
	fn := typ.Func().Returns(typ.Never).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect for never-returning function")
	}
	if !result.Terminates {
		t.Error("expected Terminates to be true for never-returning function")
	}
}

func TestEffectFromType_WithRefinement(t *testing.T) {
	eff := &constraint.FunctionEffect{Terminates: true}
	fn := typ.Func().Returns(typ.String).WithRefinement(eff).Build()
	result := EffectFromType(fn)
	if result == nil {
		t.Fatal("expected non-nil effect")
	}
	if !result.Terminates {
		t.Error("expected refinement to be returned")
	}
}

func TestEnrichExportWithEffects_NilExport(t *testing.T) {
	result := EnrichExportWithEffects(nil, "", nil, nil)
	if result != nil {
		t.Error("expected nil for nil export")
	}
}

func TestEnrichExportWithEffects_NilGraph(t *testing.T) {
	rec := typ.NewRecord().Build()
	result := EnrichExportWithEffects(rec, "", nil, nil)
	if result != rec {
		t.Error("expected original record returned when graph is nil")
	}
}

func TestEnrichExportWithEffects_EmptyEffects(t *testing.T) {
	rec := typ.NewRecord().Build()
	result := EnrichExportWithEffects(rec, "", map[cfg.SymbolID]*constraint.FunctionEffect{}, nil)
	if result != rec {
		t.Error("expected original record returned when effects map is empty")
	}
}

func TestEnrichExportWithEffects_NonRecordNonInterface(t *testing.T) {
	result := EnrichExportWithEffects(typ.String, "", map[cfg.SymbolID]*constraint.FunctionEffect{1: {}}, nil)
	if result != typ.String {
		t.Error("expected original type returned for non-record/non-interface")
	}
}
