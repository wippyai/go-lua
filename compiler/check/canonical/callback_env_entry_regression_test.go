package canonical_test

import (
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"testing"
)

func TestCanonicalCallbackEnvOverlaySeedsNestedCallbackEntryValue(t *testing.T) {
	txType := typ.NewInterface("migration.Transaction", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("sql", typ.String).
				Returns(typ.Any, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	stepFn := typ.Func().Param("db", txType).Returns(typ.Nil).Build()
	upFn := typ.Func().Param("fn", stepFn).Returns(typ.Nil).Build()
	overlay := map[string]typ.Type{}
	databaseCallbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 1},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(overlay)
	databaseFn := typ.Func().
		Param("db_type", typ.String).
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(contract.NewSpec().WithCallback(1, databaseCallbackSpec)).
		Build()
	callbackSpec := (&contract.CallbackSpec{
		InputSource: effect.ParamRef{Index: 0},
		Cardinality: contract.CardExactlyOnce,
	}).WithEnvOverlay(overlay)
	overlay["database"] = databaseFn
	overlay["up"] = upFn
	defineFn := typ.Func().
		Param("fn", typ.Func().Returns(typ.Nil).Build()).
		Returns(typ.Nil).
		Spec(contract.NewSpec().WithCallback(0, callbackSpec)).
		Build()
	moduleType := typ.NewRecord().Field("define", defineFn).Build()
	manifest := io.NewManifest("migration_lib")
	manifest.SetExport(moduleType)

	res := testutil.Check(`
		migration_lib.define(function()
			database("postgres", function()
				up(function(db)
					db:query("SELECT 1")
				end)
			end)
		end)
	`, testutil.WithStdlib(), testutil.WithManifest("migration_lib", manifest))
	if res.HasError() {
		t.Fatalf("expected clean check, got diagnostics: %v", testutil.ErrorMessages(res.Diagnostics))
	}
	fn := findFunctionWithParamNames(t, res.Session.Results, "db")
	dbSym := singleSymbolNamed(t, fn.Graph, "db")
	got := fn.NarrowedTypeAt(fn.Graph.Entry(), constraint.NewPath(dbSym, "db"))
	if !typ.TypeEquals(got, txType) {
		t.Fatalf("nested callback db entry type = %v, want %v", got, txType)
	}
}
