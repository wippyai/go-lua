package effects

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/exportkey"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/parse"
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
	fnEffect := &constraint.FunctionRefinement{
		Terminates: true,
	}
	result := Propagate(&api.FuncResult{FnRefinement: fnEffect}, nil)
	if result == nil {
		t.Fatal("expected non-nil effect")
	}
	if !result.Terminates {
		t.Error("expected Terminates to be true")
	}
}

func TestResolveRefinementBySym_NilFacts(t *testing.T) {
	result := ResolveRefinementBySym(nil, nil, nil, 1)
	if result != nil {
		t.Errorf("expected nil for nil facts, got %v", result)
	}
}

func TestResolveRefinementBySym_ZeroSym(t *testing.T) {
	result := ResolveRefinementBySym(nil, nil, nil, 0)
	if result != nil {
		t.Errorf("expected nil for zero symbol, got %v", result)
	}
}

func TestResolveRefinementBySym_GlobalOverlay(t *testing.T) {
	sym := cfg.SymbolID(7)
	bindings := bind.NewBindingTable()
	bindings.SetKind(sym, cfg.SymbolGlobal)
	bindings.SetName(sym, "send")

	row := effect.Empty.With(effect.IO{})
	globalTypes := globalenv.TypeOverlayFromMap(map[string]typ.Type{
		"send": typ.Func().Returns(typ.Nil).Effects(row).Build(),
	})
	result := ResolveRefinementBySym(nil, bindings, globalTypes, sym)
	if result == nil || result.Row == nil {
		t.Fatalf("ResolveRefinementBySym global = %#v, want effect row", result)
	}
}

func TestTerminatesFromReachability_NilResult(t *testing.T) {
	if TerminatesFromReachability(nil) {
		t.Error("expected false for nil result")
	}
}

func TestTerminatesFromReachability_NilConditionProofFacts(t *testing.T) {
	if TerminatesFromReachability(&api.FuncResult{}) {
		t.Error("expected false for nil condition proof facts")
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
	eff := &constraint.FunctionRefinement{Terminates: true}
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
	result := EnrichExportWithEffects(rec, "", map[cfg.SymbolID]*constraint.FunctionRefinement{}, nil)
	if result != rec {
		t.Error("expected original record returned when effects map is empty")
	}
}

func TestEnrichExportWithEffects_NonRecordNonInterface(t *testing.T) {
	result := EnrichExportWithEffects(typ.String, "", map[cfg.SymbolID]*constraint.FunctionRefinement{1: {}}, nil)
	if result != typ.String {
		t.Error("expected original type returned for non-record/non-interface")
	}
}

func TestExportKeyFromTargetPath(t *testing.T) {
	tests := []struct {
		name     string
		rootName string
		path     constraint.Path
		want     constraint.Segment
		wantOK   bool
	}{
		{name: "direct no root", path: constraint.Path{Root: "validate", Symbol: 1}, want: constraint.Segment{Kind: constraint.SegmentField, Name: "validate"}, wantOK: true},
		{name: "single field no root", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "validate"}}}, want: constraint.Segment{Kind: constraint.SegmentField, Name: "validate"}, wantOK: true},
		{name: "string index no root", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "validate"}}}, want: constraint.Segment{Kind: constraint.SegmentIndexString, Name: "validate"}, wantOK: true},
		{name: "int index no root", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}}}, want: constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}, wantOK: true},
		{name: "nested no root rejected", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "api"}, {Kind: constraint.SegmentField, Name: "validate"}}}, wantOK: false},
		{name: "root direct", rootName: "M", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "validate"}}}, want: constraint.Segment{Kind: constraint.SegmentField, Name: "validate"}, wantOK: true},
		{name: "root nested rejected", rootName: "M", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "api"}, {Kind: constraint.SegmentField, Name: "validate"}}}, wantOK: false},
		{name: "root mismatch rejected", rootName: "M", path: constraint.Path{Root: "N", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "validate"}}}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := exportkey.FromTargetPath(tt.rootName, tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("key=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEnrichExportWithEffects_PreservesRecordQualifiersAndMetatable(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local function validate(v)
			return v
		end
		return validate
	`, "validate")

	symValidate, ok := graph.SymbolAt(graph.Entry(), "validate")
	if !ok || symValidate == 0 {
		t.Fatal("expected symbol for validate")
	}

	fn := typ.Func().Param("v", typ.String).Returns(typ.Boolean).Build()
	meta := typ.NewRecord().Field("__index", typ.Any).Build()
	export := typ.NewRecord().
		OptReadonlyField("validate", fn).
		Field("plain", typ.Number).
		MapComponent(typ.String, typ.Integer).
		SetOpen(true).
		Metatable(meta).
		Build()

	effectsBySym := map[cfg.SymbolID]*constraint.FunctionRefinement{
		symValidate: {Terminates: true},
	}

	enriched := EnrichExportWithEffects(export, "", effectsBySym, graph)
	rec, ok := enriched.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", enriched)
	}

	field := rec.GetField("validate")
	if field == nil {
		t.Fatal("expected validate field")
	}
	if !field.Optional {
		t.Fatal("validate field optional flag should be preserved")
	}
	if !field.Readonly {
		t.Fatal("validate field readonly flag should be preserved")
	}
	fnType, ok := field.Type.(*typ.Function)
	if !ok {
		t.Fatalf("validate field should be function, got %T", field.Type)
	}
	if fnType.Refinement == nil {
		t.Fatal("validate function should be enriched with refinement")
	}

	plain := rec.GetField("plain")
	if plain == nil || plain.Optional || plain.Readonly {
		t.Fatalf("plain field flags should remain unchanged: %+v", plain)
	}
	if !rec.HasMapComponent() || !typ.TypeEquals(rec.MapKey, typ.String) || !typ.TypeEquals(rec.MapValue, typ.Integer) {
		t.Fatalf("map component should be preserved, got key=%v value=%v", rec.MapKey, rec.MapValue)
	}
	if !rec.Open {
		t.Fatal("record open flag should be preserved")
	}
	if !typ.TypeEquals(rec.Metatable, meta) {
		t.Fatalf("metatable should be preserved, got %v want %v", rec.Metatable, meta)
	}
}

func TestPropagate_CollectsEffectFromAssignmentCallSite(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local x = f()
		return x
	`, "f")

	symF, ok := graph.SymbolAt(graph.Entry(), "f")
	if !ok || symF == 0 {
		t.Fatal("expected symbol for f")
	}

	result := Propagate(&api.FuncResult{
		Graph:        graph,
		Evidence:     evidenceForEffects(graph),
		FnRefinement: &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == symF {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect from assignment call site, got %#v", result.Row)
	}
}

func TestPropagate_CollectsEffectFromReturnCallSite(t *testing.T) {
	graph := buildGraphForEffects(t, `
		return f()
	`, "f")

	symF, ok := graph.SymbolAt(graph.Entry(), "f")
	if !ok || symF == 0 {
		t.Fatal("expected symbol for f")
	}

	result := Propagate(&api.FuncResult{
		Graph:        graph,
		Evidence:     evidenceForEffects(graph),
		FnRefinement: &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == symF {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect from return call site, got %#v", result.Row)
	}
}

func TestPropagate_UsesCanonicalCandidatesWhenRawSymbolMissing(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local x = f()
		return x
	`, "f")

	symF, ok := graph.SymbolAt(graph.Entry(), "f")
	if !ok || symF == 0 {
		t.Fatal("expected symbol for f")
	}

	// Simulate missing raw call symbol. Propagation should still resolve
	// callee via canonical callsite candidates from call expression/bindings.
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			info.CalleeSymbol = 0
		}
	})

	result := Propagate(&api.FuncResult{
		Graph:        graph,
		Evidence:     evidenceForEffects(graph),
		FnRefinement: &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == symF {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect via canonical candidate lookup, got %#v", result.Row)
	}
}

func TestPropagate_UsesModuleBindingNameFallback(t *testing.T) {
	graph := buildGraphForEffects(t, `
		local x = f()
		return x
	`)

	const fallbackSym cfg.SymbolID = 777
	moduleBindings := bind.NewBindingTable()
	moduleBindings.SetName(fallbackSym, "f_alias")

	// Force callsite identity recovery through module-binding name fallback.
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			info.Callee = nil
			info.CalleeSymbol = 0
			info.CalleeName = "f_alias"
		}
	})

	result := Propagate(&api.FuncResult{
		Graph:          graph,
		ModuleBindings: moduleBindings,
		Evidence:       evidenceForEffects(graph),
		FnRefinement:   &constraint.FunctionRefinement{},
	}, func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		if sym == fallbackSym {
			return &constraint.FunctionRefinement{
				Row: effect.Row{Labels: []effect.Label{effect.IO{}}},
			}
		}
		return nil
	})

	row, ok := result.Row.(effect.Row)
	if !ok || !row.HasIO() {
		t.Fatalf("expected propagated IO effect via module-binding name fallback, got %#v", result.Row)
	}
}

func evidenceForEffects(graph *cfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	var evidence api.FlowEvidence
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			evidence.Calls = append(evidence.Calls, api.CallEvidence{Point: p, Info: info})
		}
	})
	return evidence
}

func buildGraphForEffects(t *testing.T, code string, globals ...string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, globals...)
	if graph == nil {
		t.Fatal("expected graph")
	}
	return graph
}
