package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEnvOverlay_CallExpectedArgsCarriesImportedCallbackType(t *testing.T) {
	manifest := sqlManifestWithServiceDB()
	source := `
		local sql = require("sql")

		local function up(fn: (sql.Transaction) -> any)
		end

		up(function(db)
			local result, xerr = db:execute("CREATE TABLE users(id TEXT)")
			if xerr then return end
			local changed: integer = result.rows_affected
		end)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("sql", manifest))
	root := result.Session.RootResultValue()
	if root == nil {
		t.Fatal("missing root result")
	}
	want := typ.Func().Param("db", manifest.Types["Transaction"]).Returns(typ.Any).Build()
	for callIdx, ev := range root.Evidence.Calls {
		if ev.Info == nil {
			continue
		}
		for argIdx, arg := range ev.Info.Args {
			if _, ok := arg.(*ast.FunctionExpr); !ok {
				continue
			}
			if callIdx >= len(root.CallExpectedArgs) {
				t.Fatalf("call %d has callback arg %d but no CallExpectedArgs vector; len=%d", callIdx, argIdx, len(root.CallExpectedArgs))
			}
			got := root.CallExpectedArgs[callIdx].ArgType(argIdx)
			if !typ.TypeEquals(got, want) {
				t.Fatalf("call %d arg %d expected type = %v, want %v", callIdx, argIdx, got, want)
			}
			assertCallbackDBEntryType(t, result, manifest.Types["Transaction"])
			assertCallbackExecuteResultFieldType(t, result)
			return
		}
	}
	t.Fatal("did not find callback literal call")
}

func assertCallbackDBEntryType(t *testing.T, result *testutil.Result, want typ.Type) {
	t.Helper()
	for fn, fnResult := range result.Session.Results {
		if fn == nil || fn.ParList == nil || len(fn.ParList.Names) != 1 || fn.ParList.Names[0] != "db" {
			continue
		}
		graph := fnResult.Graph
		if graph == nil || graph.Bindings() == nil {
			t.Fatal("db callback result missing graph/bindings")
		}
		var dbSymFound bool
		for _, sym := range graph.Bindings().SymbolsByName("db") {
			got := fnResult.NarrowedTypeAt(graph.Entry(), constraint.NewPath(sym, "db"))
			if typ.TypeEquals(got, want) {
				productFacts, ok := fnResult.Facts.(flow.ProductFacts)
				if !ok {
					t.Fatal("db callback facts do not expose ProductFacts")
				}
				pv := productFacts.RefinedValueAt(graph.Entry(), sym)
				if pv.State != flow.StateResolved || pv.Value.IsZero() {
					t.Fatalf("db callback entry product value unresolved for sym %d", sym)
				}
				if pv.Value.IsGradualTop() {
					t.Fatalf("db callback entry product value is gradual top; projected type=%v want concrete %v", got, want)
				}
				if projected := pv.Value.ProjectValue(); !typ.TypeEquals(projected, want) {
					t.Fatalf("db callback entry product projection = %v, want %v", projected, want)
				}
				return
			}
			t.Fatalf("db callback entry type for sym %d = %v, want %v", sym, got, want)
			dbSymFound = true
		}
		if !dbSymFound {
			t.Fatal("db callback graph has no db symbol")
		}
	}
	t.Fatal("did not find db callback result")
}

func assertCallbackExecuteResultFieldType(t *testing.T, result *testutil.Result) {
	t.Helper()
	for fn, fnResult := range result.Session.Results {
		if fn == nil || fn.ParList == nil || len(fn.ParList.Names) != 1 || fn.ParList.Names[0] != "db" {
			continue
		}
		graph := fnResult.Graph
		if graph == nil || graph.Bindings() == nil {
			t.Fatal("db callback result missing graph/bindings")
		}
		var resultSym cfg.SymbolID
		for _, sym := range graph.Bindings().SymbolsByName("result") {
			resultSym = sym
			break
		}
		if resultSym == 0 {
			t.Fatal("db callback graph has no result symbol")
		}
		var dbSym cfg.SymbolID
		for _, sym := range graph.Bindings().SymbolsByName("db") {
			dbSym = sym
			break
		}
		if dbSym == 0 {
			t.Fatal("db callback graph has no db symbol")
		}
		path := constraint.NewPath(resultSym, "result").Field("rows_affected")
		obs := observation.FromFuncResult(fnResult, nil).WithProofValues()
		var executePoint cfg.Point
		var executePostRoot typ.Type
		var executePostField typ.Type
		for _, assign := range fnResult.Evidence.Assignments {
			if assign.Info == nil {
				continue
			}
			for _, target := range assign.Info.Targets {
				if target.Kind != cfg.TargetIdent || target.Name != "result" {
					continue
				}
				src := assign.Info.LastSource()
				if _, ok := src.(*ast.FuncCallExpr); !ok {
					continue
				}
				productFacts, ok := fnResult.Facts.(flow.ProductFacts)
				if !ok {
					t.Fatal("db callback facts do not expose ProductFacts")
				}
				dbPV := productFacts.RefinedValueAt(assign.Point, dbSym)
				if dbPV.State != flow.StateResolved || dbPV.Value.IsZero() || dbPV.Value.IsGradualTop() || typ.IsAny(dbPV.Value.ProjectValue()) {
					t.Fatalf("db product at execute point %d = state %v type %v gradual=%v, want concrete", assign.Point, dbPV.State, dbPV.Value.ProjectValue(), dbPV.Value.IsGradualTop())
				}
				returns := obs.MultiTypeOf(src, assign.Point)
				if len(returns) == 0 {
					t.Fatalf("observer returns for db:execute at point %d = empty", assign.Point)
				}
				field, ok := querycore.Field(returns[0], "rows_affected")
				if !ok || !typ.TypeEquals(field, typ.Integer) {
					t.Fatalf("observer db:execute return[0].rows_affected at point %d = %v/%v, want integer; return[0]=%v", assign.Point, field, ok, returns[0])
				}
				executePoint = assign.Point
				if postFacts, ok := fnResult.Facts.(interface {
					PostRefinedPathAt(cfg.Point, constraint.Path) flow.TypedValue
				}); ok {
					executePostRoot = postFacts.PostRefinedPathAt(assign.Point, constraint.NewPath(resultSym, "result")).Type
					executePostField = postFacts.PostRefinedPathAt(assign.Point, path).Type
				}
			}
		}
		for _, assign := range fnResult.Evidence.Assignments {
			if assign.Info == nil {
				continue
			}
			for _, target := range assign.Info.Targets {
				if target.Kind != cfg.TargetIdent || target.Name != "changed" {
					continue
				}
				got := fnResult.NarrowedTypeAt(assign.Point, path)
				if !typ.TypeEquals(got, typ.Integer) {
					productFacts, _ := fnResult.Facts.(flow.ProductFacts)
					var rootProduct string
					if productFacts != nil {
						pv := productFacts.RefinedValueAt(assign.Point, resultSym)
						rootProduct = typ.FormatShort(pv.Value.ProjectValue())
						if pv.Value.IsGradualTop() {
							rootProduct += " gradual"
						}
					}
					t.Fatalf("NarrowedTypeAt(%s at changed assignment point %d) = %v, want integer; result root product=%s; execute point=%d post root=%v post field=%v; diagnostics=%v", path.String(), assign.Point, got, rootProduct, executePoint, executePostRoot, executePostField, testutil.ErrorMessages(result.Diagnostics))
				}
				return
			}
		}
		t.Fatal("db callback graph has no changed assignment")
	}
	t.Fatal("did not find db callback result")
}
