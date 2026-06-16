package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCallSignatureUsesImportedModuleRootIdentity(t *testing.T) {
	wantType := typ.Func().Param("src", typ.String).Returns(typ.Number).Build()
	m := manifest.New("json")
	m.DefineFunctionSignature("json.decode", signature.Function{Type: wantType})

	result, err := CheckChunk(parseChunk(t, `
		local json = require("json")
		local value = json.decode("{}")
	`), Config{
		Registry: standard.Registry(),
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	sig, ok := onlyCallSignature(t, result)
	if !ok {
		t.Fatalf("missing imported json.decode signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestCallSignatureDoesNotUseReassignedImportedModuleRoot(t *testing.T) {
	m := manifest.New("json")
	m.DefineFunctionSignature("json.decode", signature.Function{
		Type: typ.Func().Param("src", typ.String).Returns(typ.Number).Build(),
	})

	result, err := CheckChunk(parseChunk(t, `
		local json = require("json")
		json = {}
		local value = json.decode("{}")
	`), Config{
		Registry: standard.Registry(),
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	if sig, ok := onlyCallSignature(t, result); ok {
		t.Fatalf("signature = %v, want none after import root reassignment", sig)
	}
}

func TestCallSignatureUsesCapturedImportedModuleRootIdentity(t *testing.T) {
	wantType := typ.Func().Param("src", typ.String).Returns(typ.Number).Build()
	m := manifest.New("json")
	m.DefineFunctionSignature("json.decode", signature.Function{Type: wantType})

	stmts := parseChunk(t, `
		local json = require("json")
		local function decode()
			return json.decode("{}")
		end
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}

	result, err := CheckBoundFunction(functions[0], bindings, Config{
		Registry: standard.Registry(),
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	sig, ok := onlyCallSignature(t, result)
	if !ok {
		t.Fatalf("missing captured imported json.decode signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestCallSignatureUsesCapturedStaticMemberImportAlias(t *testing.T) {
	wantType := typ.Func().Param("url", typ.String).Returns(typ.Number).Build()
	m := manifest.New("http_client")
	m.DefineFunctionSignature("http_client.get", signature.Function{Type: wantType})

	stmts := parseChunk(t, `
		local http_client = require("http_client")
		local client = {
			_http_client = http_client,
		}
		function client.request()
			return client._http_client.get("/v1")
		end
	`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"require"}})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}

	result, err := CheckBoundFunction(functions[0], bindings, Config{
		Registry: standard.Registry(),
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	sig, ok := onlyCallSignature(t, result)
	if !ok {
		t.Fatalf("missing captured client._http_client.get signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestCallSignatureUsesSameBodyStaticMemberImportAliasAssignment(t *testing.T) {
	wantType := typ.Func().Param("url", typ.String).Returns(typ.Number).Build()
	m := manifest.New("http_client")
	m.DefineFunctionSignature("http_client.get", signature.Function{Type: wantType})

	result, err := CheckChunk(parseChunk(t, `
		local http_client = require("http_client")
		local client = {}
		client._http_client = http_client
		local value = client._http_client.get("/v1")
	`), Config{
		Registry: standard.Registry(),
		Globals:  []string{"require"},
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	sig, ok := onlyCallSignature(t, result)
	if !ok {
		t.Fatalf("missing same-body client._http_client.get signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func onlyCallSignature(t *testing.T, result *Result) (signature.Function, bool) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	var out signature.Function
	count := 0
	for _, point := range graph.RPO() {
		site, ok := result.CallSite(point)
		if !ok {
			continue
		}
		sig, ok := result.CallSignature(site)
		if !ok {
			continue
		}
		out = sig
		count++
	}
	if count > 1 {
		t.Fatalf("call signatures = %d, want at most one", count)
	}
	return out, count == 1
}
