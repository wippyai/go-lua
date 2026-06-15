package moduleidentity

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestExactRequireCallRecognizesOnlyGlobalRequireStringCalls(t *testing.T) {
	stmts := parseChunk(t, `
		local json = require("json")
		local require = function(name) return name end
		local shadowed = require("shadowed")
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})

	first := stmts[0].(*ast.LocalAssignStmt)
	modulePath, ok := ExactRequireCall(bindings, first.Exprs[0])
	if !ok || modulePath != "json" {
		t.Fatalf("ExactRequireCall(global require) = %q/%v, want json/true", modulePath, ok)
	}

	shadowed := stmts[2].(*ast.LocalAssignStmt)
	if modulePath, ok := ExactRequireCall(bindings, shadowed.Exprs[0]); ok {
		t.Fatalf("ExactRequireCall(shadowed require) = %q/true, want false", modulePath)
	}
}

func TestProjectionSharesAliasAndSignatureIdentity(t *testing.T) {
	projection, graph, sem := buildProjection(t, `
		local json = require("json")
		local value = json.decode("{}")
	`)

	modulePath, ok := projection.ModulePathForAlias("json")
	if !ok || modulePath != "json" {
		t.Fatalf("ModulePathForAlias(json) = %q/%v, want json/true", modulePath, ok)
	}

	var got string
	for _, point := range graph.RPO() {
		call, ok := sem.Call(point)
		if !ok || !call.HasCalleePath {
			continue
		}
		name, ok := projection.SignatureName(point, call.CalleePath)
		if !ok {
			continue
		}
		got = name
	}
	if got != "json.decode" {
		t.Fatalf("SignatureName = %q, want json.decode", got)
	}
}

func TestProjectionInvalidatesReassignedRequireRoot(t *testing.T) {
	projection, graph, sem := buildProjection(t, `
		local json = require("json")
		json = {}
		local value = json.decode("{}")
	`)

	for _, point := range graph.RPO() {
		call, ok := sem.Call(point)
		if !ok || !call.HasCalleePath {
			continue
		}
		if name, ok := projection.SignatureName(point, call.CalleePath); ok {
			t.Fatalf("SignatureName after reassignment = %q/true, want false", name)
		}
	}
}

func buildProjection(t *testing.T, src string) (Projection, cfg.Graph, *semantics.Result) {
	t.Helper()
	stmts := parseChunk(t, src)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	sem, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	return New(bindings, built.Graph, sem), built.Graph, sem
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "moduleidentity_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	return stmts
}
