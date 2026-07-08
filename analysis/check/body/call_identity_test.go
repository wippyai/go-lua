package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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

func TestCallSignatureTypeCachesReadOnlyFunctionType(t *testing.T) {
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

	point, ok := onlySignatureCallPointNamed(t, result, "json.decode")
	if !ok {
		t.Fatalf("missing imported json.decode call site")
	}
	first, ok := result.CallSignatureTypeAtPoint(point)
	if !ok {
		t.Fatalf("missing imported json.decode signature type")
	}
	second, ok := result.CallSignatureTypeAtPoint(point)
	if !ok {
		t.Fatalf("missing cached imported json.decode signature type")
	}
	if first != second {
		t.Fatalf("CallSignatureType returned different type pointers across reads")
	}
	if !typ.TypeEquals(first, wantType) {
		t.Fatalf("signature type = %v, want %v", first, wantType)
	}
}

func TestCallSignatureTypeUsesImplicitStdlibGlobalIdentity(t *testing.T) {
	result, err := CheckChunk(parseChunk(t, `
		local value = tostring(1)
	`), Config{
		Registry:   standard.Registry(),
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, ok := onlySignatureCallPointNamed(t, result, "tostring")
	if !ok {
		t.Fatal("missing tostring call site")
	}
	fn, ok := result.CallSignatureTypeAtPoint(point)
	if !ok {
		t.Fatal("missing implicit stdlib tostring signature type")
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("tostring signature = %v, want string return", fn)
	}
}

func TestCallSignatureTypeUsesImplicitStdlibMemberIdentity(t *testing.T) {
	result, err := CheckChunk(parseChunk(t, `
		local items: string[] = {}
		table.insert(items, "a")
	`), Config{
		Registry:   standard.Registry(),
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	point, ok := onlySignatureCallPointNamed(t, result, "table.insert")
	if !ok {
		t.Fatal("missing table.insert call site")
	}
	name, ok := result.CallSignatureNameAtPoint(point)
	if !ok || name != "table.insert" {
		t.Fatalf("signature name = %q/%v, want table.insert", name, ok)
	}
	fn, ok := result.CallSignatureTypeAtPoint(point)
	if !ok || fn == nil {
		t.Fatalf("missing implicit stdlib table.insert signature type")
	}
}

func TestCallSignatureDoesNotUseImplicitStdlibMemberAfterGlobalTableOverride(t *testing.T) {
	result, err := CheckChunk(parseChunk(t, `
		_G.coroutine = {
			spawn = function(fn: () -> ())
				return true
			end,
		}
		coroutine.spawn(function() end)
	`), Config{
		Registry:   standard.Registry(),
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	found := false
	for _, point := range result.Graph().RPO() {
		site, ok := result.CallSiteView(point)
		if !ok || site.CalleePathRef().String() != "coroutine.spawn" {
			continue
		}
		found = true
		if name, ok := result.CallSignatureNameAtPoint(point); ok {
			t.Fatalf("signature name = %q, want none after _G.coroutine override", name)
		}
		if fn, ok := result.CallSignatureTypeAtPoint(point); ok {
			t.Fatalf("signature type = %v, want none after _G.coroutine override", fn)
		}
	}
	if !found {
		t.Fatal("missing coroutine.spawn call site")
	}
}

func TestCallSignatureDoesNotUseManifestForUnresolvedImplicitGlobal(t *testing.T) {
	m := manifest.New("ambient")
	m.DefineFunctionSignature("imported", signature.Function{
		Type: typ.Func().Param("src", typ.String).Returns(typ.Number).Build(),
	})

	result, err := CheckChunk(parseChunk(t, `local value = imported(42)`), Config{
		Registry: standard.Registry(),
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	if sig, ok := onlyCallSignature(t, result); ok {
		t.Fatalf("signature = %v, want none for unresolved implicit global", sig)
	}
}

func TestCallSignatureUsesImportedStaticIntMemberIdentity(t *testing.T) {
	wantType := typ.Func().Param("src", typ.String).Returns(typ.Number).Build()
	m := manifest.New("pkg")
	m.DefineFunctionSignature("pkg[1]", signature.Function{Type: wantType})

	result, err := CheckChunk(parseChunk(t, `
		local pkg = require("pkg")
		local value = pkg[1]("payload")
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
		t.Fatalf("missing imported pkg[1] signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestCallSignatureUsesImportedStaticIntMemberAlias(t *testing.T) {
	wantType := typ.Func().Param("src", typ.String).Returns(typ.Number).Build()
	m := manifest.New("pkg")
	m.DefineFunctionSignature("pkg[1]", signature.Function{Type: wantType})

	result, err := CheckChunk(parseChunk(t, `
		local pkg = require("pkg")
		local alias = pkg
		local value = alias[1]("payload")
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
		t.Fatalf("missing imported alias[1] signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestCallSignatureUsesNestedImportedStaticIntMemberIdentity(t *testing.T) {
	wantType := typ.Func().Param("src", typ.String).Returns(typ.Number).Build()
	m := manifest.New("pkg")
	m.DefineFunctionSignature("pkg.sub[1]", signature.Function{Type: wantType})

	result, err := CheckChunk(parseChunk(t, `
		local pkg = require("pkg")
		local value = pkg.sub[1]("payload")
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
		t.Fatalf("missing imported pkg.sub[1] signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestCallSignatureDoesNotCollapseDynamicIndexToStaticIntMember(t *testing.T) {
	m := manifest.New("pkg")
	m.DefineFunctionSignature("pkg[1]", signature.Function{
		Type: typ.Func().Param("src", typ.String).Returns(typ.Number).Build(),
	})

	result, err := CheckChunk(parseChunk(t, `
		local pkg = require("pkg")
		local index = 1
		local value = pkg[index]("payload")
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
		t.Fatalf("signature = %v, want none for dynamic index", sig)
	}
}

func TestCallSignatureDoesNotCollapseStringIndexToStaticIntMember(t *testing.T) {
	m := manifest.New("pkg")
	m.DefineFunctionSignature("pkg[1]", signature.Function{
		Type: typ.Func().Param("src", typ.String).Returns(typ.Number).Build(),
	})

	result, err := CheckChunk(parseChunk(t, `
		local pkg = require("pkg")
		local value = pkg["1"]("payload")
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
		t.Fatalf("signature = %v, want none for string index when only pkg[1] exists", sig)
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

func TestCallSignatureUsesLocalImportedFunctionAlias(t *testing.T) {
	wantType := typ.Func().Param("payload", typ.Any).Build()
	m := manifest.New("runtime")
	m.DefineFunctionSignature("runtime.send", signature.Function{Type: wantType})

	result, err := CheckChunk(parseChunk(t, `
		local runtime = require("runtime")
		local send = runtime.send
		send({})
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
		t.Fatalf("missing local send alias signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestCallSignatureUsesCapturedLocalImportedFunctionAlias(t *testing.T) {
	wantType := typ.Func().Param("payload", typ.Any).Build()
	m := manifest.New("runtime")
	m.DefineFunctionSignature("runtime.send", signature.Function{Type: wantType})

	stmts := parseChunk(t, `
		local runtime = require("runtime")
		local send = runtime.send
		local function invoke()
			send({})
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
		t.Fatalf("missing captured local send alias signature")
	}
	if !typ.TypeEquals(sig.Type, wantType) {
		t.Fatalf("signature type = %v, want %v", sig.Type, wantType)
	}
}

func TestIteratorIdentityAllowsImplicitBuiltinPairs(t *testing.T) {
	result, err := CheckChunk(parseChunk(t, `
local t = {}
for _, value in pairs(t) do
end
`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var names []string
	for _, point := range result.Graph().RPO() {
		site, ok := result.CallSiteView(point)
		if !ok {
			continue
		}
		name, ok := result.signatureID.nameForIndexedIteratorCallSiteView(site)
		if ok {
			names = append(names, name)
		}
	}
	if len(names) != 1 || names[0] != "pairs" {
		t.Fatalf("iterator names = %v, want [pairs]", names)
	}
}

func onlyCallSignature(t *testing.T, result *Result) (signature.Function, bool) {
	t.Helper()
	point, ok := onlySignatureCallPoint(t, result)
	if !ok {
		return signature.Function{}, false
	}
	return result.CallSignatureAtPoint(point)
}

func onlySignatureCallPoint(t *testing.T, result *Result) (cfg.Point, bool) {
	t.Helper()
	return onlySignatureCallPointNamed(t, result, "")
}

func onlySignatureCallPointNamed(t *testing.T, result *Result, wantName string) (cfg.Point, bool) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	var out cfg.Point
	count := 0
	for _, point := range graph.RPO() {
		if _, ok := result.CallSignatureAtPoint(point); !ok {
			continue
		}
		if wantName != "" {
			name, ok := result.CallSignatureNameAtPoint(point)
			if !ok || name != wantName {
				continue
			}
		}
		out = point
		count++
	}
	if count > 1 {
		t.Fatalf("call sites = %d, want at most one", count)
	}
	return out, count == 1
}
