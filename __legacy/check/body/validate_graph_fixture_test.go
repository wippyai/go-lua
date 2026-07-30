package body

import (
	"os"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func validateGraphPreparedFixture(t testing.TB) *Static {
	t.Helper()
	src, err := os.ReadFile("../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	stmts := parseChunk(t, string(src))
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"uuid"}})
	var target bind.FunctionOrigin
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func.Line() == 743 {
			target = origin
			break
		}
	}
	if target.Func == nil {
		t.Fatal("compiler.validate_graph function at line 743 is missing")
	}
	uuid := manifest.New("uuid")
	uuid.SetExport(typetable.NewRecord().Field("v7", typ.Func().Returns(typ.String).Build()).Build())
	prepared, err := PrepareBoundFunction(target.Func, bindings, Config{
		Registry: standard.Registry(), Globals: []string{"uuid"},
		Signatures:    signaturelookup.Source{IncludeStdlib: true, Manifests: []*manifest.Manifest{uuid}},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{uuid}},
	})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	if prepared.operationPlan == nil {
		t.Fatal("prepared function has no factflow operation plan")
	}
	return prepared
}
