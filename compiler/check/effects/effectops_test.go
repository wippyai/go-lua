package effects

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/exportkey"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

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
