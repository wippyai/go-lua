package body

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLuaTypeIntrinsicIdentityIsCanonicalGlobalAndShadowSafe(t *testing.T) {
	config := Config{Registry: standard.Registry(), Signatures: signaturelookup.Source{IncludeStdlib: true}}
	tests := []struct {
		name      string
		src       string
		want      bool
		configure func(*Config)
	}{
		{name: "canonical", src: `function f(value) return type(value) end`, want: true},
		{name: "local-shadow", src: `function f(value) local type = function() return "shadow" end return type(value) end`},
		{name: "global-replaced", src: `function f(value) _G.type = function() return "shadow" end return type(value) end`},
		{name: "conditional-direct-global-write", src: `function f(value) if value then type = function() return "shadow" end end return type(value) end`},
		{name: "conditional-global-table-write", src: `function f(value) if value then _G.type = function() return "shadow" end end return type(value) end`},
		{name: "conditional-global-table-index-write", src: `function f(value) if value then _G["type"] = function() return "shadow" end end return type(value) end`},
		{name: "dynamic-global-table-write", src: `function f(value, key) if value then _G[key] = function() return "shadow" end end return type(value) end`},
		{name: "global-table-alias-write", src: `function f(value) local globals = _G if value then globals.type = function() return "shadow" end end return type(value) end`},
		{name: "global-table-alias-chain-dynamic-write", src: `function f(value, key) local globals = _G local alias = globals if value then alias[key] = function() return "shadow" end end return type(value) end`},
		{name: "global-table-call-escape", src: `function f(value) mutate(_G) return type(value) end`},
		{name: "global-table-container-escape", src: `function f(value) local container = {_G} return type(value) end`},
		{name: "global-table-rawset", src: `function f(value) rawset(_G, "other", true) return type(value) end`},
		{name: "harmless-global-table-read", src: `function f(value) local globals = _G return type(value) end`},
		{name: "typed-global-override", src: `function f(value) return type(value) end`, configure: func(config *Config) {
			config.GlobalTypes = map[string]typ.Type{"type": typ.Func().Param("value", typ.Any).Returns(typ.Number).Build()}
		}},
		{name: "member", src: `function f(value) local object = { type = function() return "member" end } return object.type(value) end`},
		{name: "method", src: `function f(value) local object = { type = function(self) return "method" end } return object:type(value) end`},
		{name: "alias", src: `function f(value) local kind = type return kind(value) end`},
		{name: "import-alias", src: `function f(value) local imported = require("type") return imported(value) end`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			caseConfig := config
			if tc.configure != nil {
				tc.configure(&caseConfig)
			}
			prepared, err := PrepareFunction(parseFunction(t, tc.src), caseConfig)
			if err != nil {
				t.Fatalf("PrepareFunction: %v", err)
			}
			got := false
			for point := cfg.Point(0); int(point) < prepared.operationPlan.PointCount(); point++ {
				op, ok := prepared.operationPlan.SignatureCallOperation(point)
				if !ok {
					continue
				}
				intrinsic, exact := op.Intrinsic()
				if exact {
					if intrinsic != signature.IntrinsicLuaType {
						t.Fatalf("intrinsic = %d", intrinsic)
					}
					got = true
				}
			}
			if got != tc.want {
				t.Fatalf("Lua type intrinsic = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLuaTypeIntrinsicRejectsWritesFromSiblingFunctionInBoundUnit(t *testing.T) {
	config := Config{Registry: standard.Registry(), Signatures: signaturelookup.Source{IncludeStdlib: true}}
	tests := []struct {
		name  string
		write string
	}{
		{name: "direct", write: `type = replacement`},
		{name: "global-table-static", write: `_G.type = replacement`},
		{name: "global-table-dynamic", write: `_G[key] = replacement`},
		{name: "global-table-alias", write: `local globals = _G; globals.type = replacement`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmts := parseChunk(t, `
				function mutate(key) `+tc.write+` end
				function target(value) return type(value) end
			`)
			bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(config)})
			functions := bindings.Functions()
			if len(functions) != 2 {
				t.Fatalf("bound functions = %d, want 2", len(functions))
			}
			prepared, err := PrepareBoundFunction(functions[1], bindings, config)
			if err != nil {
				t.Fatal(err)
			}
			for point := cfg.Point(0); int(point) < prepared.operationPlan.PointCount(); point++ {
				if op, ok := prepared.operationPlan.SignatureCallOperation(point); ok {
					if intrinsic, exact := op.Intrinsic(); exact {
						t.Fatalf("sibling write admitted intrinsic %d", intrinsic)
					}
				}
			}
		})
	}
}

func TestLuaTypeIntrinsicGlobalTableReadsRejectRegardlessOfTraversalOrder(t *testing.T) {
	config := Config{Registry: standard.Registry(), Signatures: signaturelookup.Source{IncludeStdlib: true}}
	tests := []struct {
		name string
		src  string
	}{
		{name: "backward-sibling", src: `
			local globals
			function mutate() globals.type = replacement end
			globals = _G
			function target(value) return type(value) end
		`},
		{name: "reverse-alias-chain", src: `
			local first, second
			function mutate() first.type = replacement end
			first = second
			second = _G
			function target(value) return type(value) end
		`},
		{name: "backward-nested", src: `
			function target(value)
				local globals
				local function mutate() globals.type = replacement end
				globals = _G
				return type(value)
			end
		`},
		{name: "dynamic-reverse-chain", src: `
			local first, second
			function mutate(key) first[key] = replacement end
			second = _G
			first = second
			function target(value) return type(value) end
		`},
		{name: "sibling-return-escape", src: `
			function expose() return _G end
			function target(value) return type(value) end
		`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmts := parseChunk(t, tc.src)
			bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(config)})
			functions := bindings.Functions()
			var target *ast.FunctionExpr
			for _, fn := range functions {
				origin, ok := bindings.FunctionOrigin(fn)
				if ok && origin.HasTargetSymbol && bindings.Name(origin.TargetSymbol) == "target" {
					target = fn
				}
			}
			if target == nil {
				t.Fatal("target function not found")
			}
			prepared, err := PrepareBoundFunction(target, bindings, config)
			if err != nil {
				t.Fatal(err)
			}
			for point := cfg.Point(0); int(point) < prepared.operationPlan.PointCount(); point++ {
				if op, ok := prepared.operationPlan.SignatureCallOperation(point); ok {
					if intrinsic, exact := op.Intrinsic(); exact {
						t.Fatalf("order-dependent alias census admitted intrinsic %d", intrinsic)
					}
				}
			}
		})
	}
}
