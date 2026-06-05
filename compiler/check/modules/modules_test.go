package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/exportkey"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestConnect_CreatesManifest(t *testing.T) {
	database := db.New()
	name := "test_module"
	exportType := typ.NewRecord().Build()

	manifest := Connect(database, name, exportType, nil, nil, nil)
	if manifest == nil {
		t.Fatal("expected manifest to be created")
	}
}

func TestConnect_WithExportTypes(t *testing.T) {
	database := db.New()
	name := "test_module"
	exportType := typ.NewRecord().Build()
	exportTypes := map[string]typ.Type{
		"CustomType": typ.String,
	}

	manifest := Connect(database, name, exportType, exportTypes, nil, nil)
	if manifest == nil {
		t.Fatal("expected manifest to be created")
	}
}

func TestDisconnect_RemovesManifest(t *testing.T) {
	database := db.New()
	name := "test_module"
	manifest := io.NewManifest(name)
	database.Connect(name, manifest)

	Disconnect(database, name)
}

func TestExportFunctionSummaries_NilGraph(t *testing.T) {
	manifest := io.NewManifest("test")
	ExportFunctionSummaries(manifest, typ.NewRecord().Build(), nil, nil)
}

func TestExportFunctionSummaries_EmptyEffects(t *testing.T) {
	manifest := io.NewManifest("test")
	ExportFunctionSummaries(manifest, typ.NewRecord().Build(), nil, make(map[cfg.SymbolID]*constraint.FunctionRefinement))
}

func TestExportFunctionSummaries_NonRecordExportType(t *testing.T) {
	manifest := io.NewManifest("test")
	ExportFunctionSummaries(manifest, typ.String, nil, nil)
}

func TestExportFunctionSummaries_ExportsRowOnlyEffects(t *testing.T) {
	graph := buildModulesGraph(t, `
		function M.map(info)
			return info
		end
		return M
	`, "M")
	var fnSym cfg.SymbolID
	graph.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info != nil && len(info.TargetPath.Segments) == 1 && info.TargetPath.Segments[0].Name == "map" {
			fnSym = info.Symbol
		}
	})
	if fnSym == 0 {
		t.Fatal("expected function symbol for M.map")
	}
	exportType := typ.NewRecord().
		Field("map", typ.Func().Param("info", typ.Any).Returns(typ.Any).Build()).
		Build()
	row := effect.Row{Labels: []effect.Label{
		effect.FlowInto{ParamIndex: 0, SourcePath: effect.FieldPath("message"), ReturnIndex: 0, TargetPath: effect.FieldPath("error_message")},
	}}
	manifest := io.NewManifest("test")

	ExportFunctionSummaries(manifest, exportType, graph, map[cfg.SymbolID]*constraint.FunctionRefinement{
		fnSym: {Row: row},
	})

	summary, ok := manifest.LookupSummary("map")
	if !ok {
		t.Fatal("expected row-only summary to be exported")
	}
	if !summary.Effects.Equals(row) {
		t.Fatalf("summary effects = %#v, want %#v", summary.Effects, row)
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
		{name: "direct field", path: constraint.Path{Root: "validate", Symbol: 1}, want: constraint.Segment{Kind: constraint.SegmentField, Name: "validate"}, wantOK: true},
		{name: "root field", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "validate"}}}, want: constraint.Segment{Kind: constraint.SegmentField, Name: "validate"}, wantOK: true},
		{name: "root string index", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "validate"}}}, want: constraint.Segment{Kind: constraint.SegmentIndexString, Name: "validate"}, wantOK: true},
		{name: "root int index", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}}}, want: constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}, wantOK: true},
		{name: "nested path rejected", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "api"}, {Kind: constraint.SegmentField, Name: "validate"}}}, wantOK: false},
		{name: "empty rejected", path: constraint.Path{}, wantOK: false},
		{name: "root filter accepted", rootName: "M", path: constraint.Path{Root: "M", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "validate"}}}, want: constraint.Segment{Kind: constraint.SegmentField, Name: "validate"}, wantOK: true},
		{name: "root mismatch rejected", rootName: "M", path: constraint.Path{Root: "N", Symbol: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "validate"}}}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := exportkey.FromTargetPath(tt.rootName, tt.path)
			if gotOK != tt.wantOK {
				t.Fatalf("ok=%v, want %v", gotOK, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("key=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExportMemberPathFromTargetPathPreservesNestedSegments(t *testing.T) {
	path, ok := exportkey.MemberPathFromTargetPath("M", constraint.Path{
		Root:   "M",
		Symbol: 1,
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "api"},
			{Kind: constraint.SegmentIndexString, Name: "run"},
		},
	})
	if !ok {
		t.Fatal("expected nested export member path")
	}
	segments := path.Segments()
	if len(segments) != 2 {
		t.Fatalf("segments len = %d, want 2", len(segments))
	}
	if segments[0] != (constraint.Segment{Kind: constraint.SegmentField, Name: "api"}) {
		t.Fatalf("segment[0] = %#v", segments[0])
	}
	if segments[1] != (constraint.Segment{Kind: constraint.SegmentIndexString, Name: "run"}) {
		t.Fatalf("segment[1] = %#v", segments[1])
	}
	if path.Key() == "" {
		t.Fatal("expected stable member path key")
	}
}

func TestApplyExportFunctionOverlaysUsesExactStructuralPath(t *testing.T) {
	base := typ.Func().Returns(typ.Unknown).Build()
	topRun := typ.Func().Returns(typ.String).Build()
	nestedRun := typ.Func().Returns(typ.Number).Build()
	export := typ.NewRecord().
		Field("run", base).
		Field("api", typ.NewRecord().Field("run", base).Build()).
		Build()

	runPath := mustExportMemberPath(t, "run")
	apiRunPath := mustExportMemberPath(t, "api", "run")
	got := applyExportFunctionOverlays(export, exportFunctionOverlays{
		runPath.Key():    {path: runPath, fn: topRun},
		apiRunPath.Key(): {path: apiRunPath, fn: nestedRun},
	})

	rec, ok := unwrap.Alias(got).(*typ.Record)
	if !ok {
		t.Fatalf("overlay result = %T, want record", got)
	}
	runField := rec.GetField("run")
	if runField == nil || !typ.TypeEquals(runField.Type, topRun) {
		t.Fatalf("top-level run = %v, want %v", runField, topRun)
	}
	apiField := rec.GetField("api")
	if apiField == nil {
		t.Fatalf("overlay dropped api field: %v", rec)
	}
	apiRec, ok := unwrap.Alias(apiField.Type).(*typ.Record)
	if !ok {
		t.Fatalf("api field = %T, want record", apiField.Type)
	}
	apiRun := apiRec.GetField("run")
	if apiRun == nil || !typ.TypeEquals(apiRun.Type, nestedRun) {
		t.Fatalf("api.run = %v, want %v", apiRun, nestedRun)
	}
}

func TestApplyExportFunctionOverlaysDoesNotRewriteSameLeafElsewhere(t *testing.T) {
	base := typ.Func().Returns(typ.Unknown).Build()
	topRun := typ.Func().Returns(typ.String).Build()
	export := typ.NewRecord().
		Field("run", base).
		Field("api", typ.NewRecord().Field("run", base).Build()).
		Build()

	runPath := mustExportMemberPath(t, "run")
	got := applyExportFunctionOverlays(export, exportFunctionOverlays{
		runPath.Key(): {path: runPath, fn: topRun},
	})

	rec, ok := unwrap.Alias(got).(*typ.Record)
	if !ok {
		t.Fatalf("overlay result = %T, want record", got)
	}
	apiField := rec.GetField("api")
	if apiField == nil {
		t.Fatalf("overlay dropped api field: %v", rec)
	}
	apiRec, ok := unwrap.Alias(apiField.Type).(*typ.Record)
	if !ok {
		t.Fatalf("api field = %T, want record", apiField.Type)
	}
	apiRun := apiRec.GetField("run")
	if apiRun == nil || !typ.TypeEquals(apiRun.Type, base) {
		t.Fatalf("api.run = %v, want original %v", apiRun, base)
	}
}

func TestEnrichExportFunctionsAttachesCanonicalReturnRelations(t *testing.T) {
	base := typ.Func().
		Param("ok", typ.Boolean).
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.String)).
		Build()
	relations := flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{
		{ValueIndex: 0, ErrorIndex: 1},
	})
	if direct := attachExportReturnRelations(base, relations); !erreffect.HasErrorReturnLabel(direct) {
		t.Fatalf("direct relation attachment failed: %v", direct)
	}
	export := typ.NewRecord().Field("request", base).Build()
	got := EnrichExportFunctions(export, []ExportFunctionResult{{
		TargetPath: constraint.Path{
			Root:   "M",
			Symbol: 1,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "request"},
			},
		},
		Result: &api.FuncResult{
			ReturnRelations: relations,
		},
	}})

	rec, ok := unwrap.Alias(got).(*typ.Record)
	if !ok {
		t.Fatalf("enriched export = %T, want record", got)
	}
	field := rec.GetField("request")
	if field == nil {
		t.Fatal("missing request field")
	}
	fn := unwrap.Function(field.Type)
	if fn == nil || !erreffect.HasErrorReturnLabel(fn) {
		t.Fatalf("request function missing ErrorReturn label: %v", field.Type)
	}
}

func TestEnrichExportFunctionsPreservesClosedDeclaredUnionReturns(t *testing.T) {
	accepted := typ.NewAlias("Accepted", typ.NewRecord().
		Field("id", typ.String).
		Field("attempt", typ.Number).
		Build())
	rejected := typ.NewAlias("Rejected", typ.NewRecord().
		Field("id", typ.String).
		Field("reason", typ.String).
		Build())
	decision := typ.NewAlias("Decision", typ.NewUnion(accepted, rejected))
	coalesced := typ.NewRecord().
		Field("id", typ.String).
		OptField("attempt", typ.Number).
		Field("reason", typ.Nil).
		Build()
	base := typ.Func().Returns(coalesced).Build()
	export := typ.NewRecord().Field("decide", base).Build()
	got := EnrichExportFunctions(export, []ExportFunctionResult{{
		TargetPath: constraint.Path{
			Root:   "M",
			Symbol: 1,
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentField, Name: "decide"},
			},
		},
		Result: &api.FuncResult{
			SourceSignature: typ.Func().Returns(decision).Build(),
		},
	}})

	rec, ok := unwrap.Alias(got).(*typ.Record)
	if !ok {
		t.Fatalf("enriched export = %T, want record", got)
	}
	field := rec.GetField("decide")
	if field == nil {
		t.Fatal("missing decide field")
	}
	fn := unwrap.Function(field.Type)
	if fn == nil || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], decision) {
		t.Fatalf("decide function = %v, want return %v", field.Type, decision)
	}
}

func mustExportMemberPath(t *testing.T, names ...string) exportkey.MemberPath {
	t.Helper()
	segments := make([]fieldkey.Key, 0, len(names))
	for _, name := range names {
		key, ok := fieldkey.FromName(name)
		if !ok {
			t.Fatalf("bad field key %q", name)
		}
		segments = append(segments, key)
	}
	path, ok := exportkey.NewMemberPath(segments)
	if !ok {
		t.Fatalf("bad member path %v", names)
	}
	return path
}

func buildModulesGraph(t *testing.T, code string, globals ...string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: stmts}, globals...)
	if graph == nil {
		t.Fatal("expected graph")
	}
	return graph
}
