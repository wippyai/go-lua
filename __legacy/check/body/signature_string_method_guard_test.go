package body

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestSignatureCallOperationRequiresExactDominatingStringGuard(t *testing.T) {
	positive := `
local function target(value: any): string
  if type(value) ~= "string" then return "" end
  return (value:gsub("x", "y"))
end
return target
`
	if !preparedTargetHasGsubSignature(t, positive) {
		t.Fatal("exact dominating string guard did not seal string.gsub")
	}

	for name, source := range map[string]string{
		"call before guard":     `local function target(value: any): string local out = value:gsub("x", "y") if type(value) ~= "string" then return "" end return out end return target`,
		"inverted guard":        `local function target(value: any): string if type(value) == "string" then return "" end return (value:gsub("x", "y")) end return target`,
		"alternate receiver":    `local function target(value: any, other: any): string if type(other) ~= "string" then return "" end return (value:gsub("x", "y")) end return target`,
		"join":                  `local function target(value: any, flag: boolean): string if flag then if type(value) ~= "string" then return "" end end return (value:gsub("x", "y")) end return target`,
		"alternate predecessor": `local function target(value: any, flag: boolean): string if type(value) ~= "string" then if flag then return "" end end return (value:gsub("x", "y")) end return target`,
		"root version drift":    `local function target(value: any): string if type(value) ~= "string" then return "" end value = "changed" return (value:gsub("x", "y")) end return target`,
		"captured mutation":     `local function target(value: any): string if type(value) ~= "string" then return "" end local function mutate() value = false end mutate() return (value:gsub("x", "y")) end return target`,
		"unsealed type":         `local type = function(value: any): string return "string" end local function target(value: any): string if type(value) ~= "string" then return "" end return (value:gsub("x", "y")) end return target`,
	} {
		t.Run(name, func(t *testing.T) {
			if preparedTargetHasGsubSignature(t, source) {
				t.Fatal("unproven string receiver gained signature authority")
			}
		})
	}
}

func TestSignatureCallOperationAcceptsOnlyExactImmutableStringBoundaryParam(t *testing.T) {
	positive := `
local function target(id: string): boolean
  return id:match("^__") ~= nil
end
return target
`
	if !preparedTargetHasMethodSignature(t, positive, "match") {
		t.Fatal("exact string boundary parameter did not seal string.match")
	}

	for name, source := range map[string]string{
		"any contract":      `local function target(id: any): boolean return id:match("^__") ~= nil end return target`,
		"optional contract": `local function target(id: string?): boolean return id:match("^__") ~= nil end return target`,
		"root write":        `local function target(id: string): boolean id = "changed" return id:match("^__") ~= nil end return target`,
		"captured mutation": `local function target(id: string): boolean local function mutate() id = "changed" end mutate() return id:match("^__") ~= nil end return target`,
		"alternate root":    `local function target(id: string, other: any): boolean return other:match("^__") ~= nil end return target`,
		"descendant path":   `local function target(id: { value: string }): boolean return id.value:match("^__") ~= nil end return target`,
		"dynamic pattern":   `local function target(id: string, pattern: string): boolean return id:match(pattern) ~= nil end return target`,
		"capture pattern":   `local function target(id: string): boolean return id:match("(__)") ~= nil end return target`,
		"multiple targets":  `local function target(id: string): boolean local first, second = id:match("^__") return first ~= nil end return target`,
	} {
		t.Run(name, func(t *testing.T) {
			if preparedTargetHasMethodSignature(t, source, "match") {
				t.Fatal("unproven boundary receiver gained string.match authority")
			}
		})
	}
}

func TestStaticScalarSignatureGateRejectsContextualDescriptors(t *testing.T) {
	reg := standard.Registry()
	tp := typ.NewTypeParam("T", nil)
	for name, sig := range map[string]signature.Function{
		"effectful":        {Type: typ.Func().Returns(typ.String).Build(), OperationalEffects: &signature.OperationalEffects{MaySuspend: true}},
		"generic":          {Type: typ.Func().TypeParamRef(tp).Returns(tp).Build()},
		"composite return": {Type: typ.Func().Returns(typ.NewArray(typ.String)).Build()},
	} {
		t.Run(name, func(t *testing.T) {
			if staticScalarSignature(reg, sig) {
				t.Fatal("contextual signature descriptor passed static scalar gate")
			}
		})
	}
	if !staticScalarSignature(reg, signature.Function{Type: typ.Func().Returns(typ.String, typ.Integer).Build()}) {
		t.Fatal("effect-free scalar signature was rejected")
	}
}

func preparedTargetHasGsubSignature(t *testing.T, source string) bool {
	return preparedTargetHasMethodSignature(t, source, "gsub")
}

func preparedTargetHasMethodSignature(t *testing.T, source, method string) bool {
	t.Helper()
	stmts, err := parse.ParseString(source, "guarded-string-method.lua")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		Registry: standard.Registry(), TypeValues: typevalue.NewCache(),
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte(source)),
		Signatures:    signaturelookup.Source{IncludeStdlib: true},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: Globals(config)})
	functions := bindings.Functions()
	if len(functions) == 0 {
		t.Fatal("fixture has no bound functions")
	}
	target := functions[0]
	for _, origin := range bindings.FunctionOrigins() {
		if origin.HasTargetSymbol && bindings.Name(origin.TargetSymbol) == "target" {
			target = origin.Func
			break
		}
	}
	if target == nil {
		t.Fatal("target function not found")
	}
	prepared, err := PrepareBoundFunction(target, bindings, config)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	found := false
	for raw := 0; raw < plan.PointCount(); raw++ {
		point := cfg.Point(raw)
		site, ok := plan.Facts().CallSiteView(point)
		if !ok || site.MethodName() != method {
			continue
		}
		if found {
			t.Fatalf("fixture contains multiple %s sites", method)
		}
		found = true
		if _, exact := plan.SignatureCallOperation(point); exact {
			return true
		}
	}
	if !found {
		t.Fatalf("%s site not found", method)
	}
	return false
}
