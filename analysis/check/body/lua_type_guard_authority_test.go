package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLuaTypeGuardFactsRequireSealedIntrinsicEnvironment(t *testing.T) {
	base := Config{Registry: standard.Registry(), Signatures: signaturelookup.Source{IncludeStdlib: true}}
	tests := []struct {
		name      string
		src       string
		wantSeal  bool
		configure func(*Config)
	}{
		{name: "canonical", wantSeal: true, src: `function target(value) if type(value) == "string" then return value end end`},
		{name: "direct-write", src: `function target(value) type = replacement if type(value) == "string" then return value end end`},
		{name: "conditional-write", src: `function target(value, flag) if flag then type = replacement end if type(value) == "string" then return value end end`},
		{name: "global-table-write", src: `function target(value) _G.type = replacement if type(value) == "string" then return value end end`},
		{name: "global-table-index-write", src: `function target(value) _G["type"] = replacement if type(value) == "string" then return value end end`},
		{name: "dynamic-global-table-write", src: `function target(value, key) _G[key] = replacement if type(value) == "string" then return value end end`},
		{name: "sibling-write", src: `function mutate() type = replacement end function target(value) if type(value) == "string" then return value end end`},
		{name: "nested-alias-write", src: `local globals = _G function mutate() globals.type = replacement end function target(value) if type(value) == "string" then return value end end`},
		{name: "global-table-call-escape", src: `function expose() mutate(_G) end function target(value) if type(value) == "string" then return value end end`},
		{name: "global-table-container-escape", src: `function expose() return {_G} end function target(value) if type(value) == "string" then return value end end`},
		{name: "global-table-return-escape", src: `function expose() return _G end function target(value) if type(value) == "string" then return value end end`},
		{name: "global-table-rawset", src: `function expose() rawset(_G, "other", true) end function target(value) if type(value) == "string" then return value end end`},
		{name: "typed-override", src: `function target(value) if type(value) == "string" then return value end end`, configure: func(c *Config) {
			c.GlobalTypes = map[string]typ.Type{"type": typ.Func().Param("value", typ.Any).Returns(typ.Number).Build()}
		}},
		{name: "stdlib-disabled", src: `function target(value) if type(value) == "string" then return value end end`, configure: func(c *Config) {
			c.Signatures = signaturelookup.Source{}
			c.Globals = []string{"type"}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := base
			if tc.configure != nil {
				tc.configure(&config)
			}
			stmts := parseChunk(t, tc.src)
			bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(config)})
			var target *ast.FunctionExpr
			for _, fn := range bindings.Functions() {
				origin, ok := bindings.FunctionOrigin(fn)
				if ok && origin.HasTargetSymbol && bindings.Name(origin.TargetSymbol) == "target" {
					target = fn
				}
			}
			if target == nil {
				t.Fatal("target function not found")
			}
			result, err := CheckBoundFunction(target, bindings, config)
			if err != nil {
				t.Fatalf("CheckBoundFunction: %v", err)
			}
			if result.sealedLuaTypeChecks != tc.wantSeal {
				t.Fatalf("stored type seal = %v, want %v", result.sealedLuaTypeChecks, tc.wantSeal)
			}

			var rawTypePoints []cfg.Point
			for point := cfg.Point(0); int(point) < result.Graph().Size(); point++ {
				result.wir.ForEachBranchCheck(point, func(check wir.Check) bool {
					if check.Kind == wir.CheckTypeEqual || check.Kind == wir.CheckTypeNot {
						rawTypePoints = append(rawTypePoints, point)
					}
					return true
				})
			}
			if len(rawTypePoints) == 0 {
				t.Fatal("test did not preserve a syntactic WIR type check")
			}
			for _, point := range rawTypePoints {
				check, published := result.BranchConditionCheck(point)
				if published != tc.wantSeal {
					t.Fatalf("BranchConditionCheck(%d) = %#v/%v, seal=%v", point, check, published, tc.wantSeal)
				}
				refinements := result.facts.BranchRefinements(point)
				evidence := result.facts.BranchPathEvidence(point)
				if tc.wantSeal && (len(refinements) == 0 || len(evidence) == 0) {
					t.Fatalf("sealed predicate facts at %d = refinements %d evidence %d", point, len(refinements), len(evidence))
				}
				if !tc.wantSeal && (len(refinements) != 0 || len(evidence) != 0) {
					t.Fatalf("unsealed predicate published facts at %d: refinements %#v evidence %#v", point, refinements, evidence)
				}
			}
		})
	}
}

func TestLuaTypeGuardImpliedChecksFilterOnlyUnsealedTypeSemantics(t *testing.T) {
	stmts := parseChunk(t, `function target(ok, value) return ok and type(value) == "string" end`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	fn := bindings.Functions()[0]
	result, err := CheckBoundFunction(fn, bindings, Config{Registry: standard.Registry(), Globals: []string{"type"}})
	if err != nil {
		t.Fatal(err)
	}
	ret := fn.Stmts[0].(*ast.ReturnStmt)
	checks := result.ExpressionImpliedChecksOnEdge(ret.Exprs[0], true)
	if len(checks) != 1 || checks[0].Check.Kind != branchcond.CheckTruthy {
		t.Fatalf("unsealed implied checks = %#v, want only ordinary truthiness", checks)
	}
}
