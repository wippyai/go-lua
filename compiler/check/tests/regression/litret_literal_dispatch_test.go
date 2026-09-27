package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestLitretLiteralParamMakesNilReturnDead(t *testing.T) {
	source := `
local http = { response = function(): string return "ok" end }
local function mod(name: "http")
    if name == "http" then return http end
    return nil
end
local h = mod("http")
local res: string = h.response()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors when param is literal \"http\" and nil return is dead")
	}
}

func TestLitretLiteralDispatchReturnSpec(t *testing.T) {
	source := `
local http = { response = function(): string return "ok" end }
local json = { encode = function(v: string): string return v end }
local function mod(name)
    if name == "http" then return http end
    if name == "json" then return json end
    return nil
end
local h = mod("http")
local res: string = h.response()
local direct: string = mod("http").response()
local encoded: string = mod("json").encode("x")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for mod(\"http\").response() via body-derived return case")
	}
}

func TestLitretLiteralDispatchDefaultStaysOptional(t *testing.T) {
	source := `
local http = { response = function(): string return "ok" end }
local json = { encode = function(v: string): string return v end }
local function mod(name)
    if name == "http" then return http end
    if name == "json" then return json end
    return nil
end
local function use(x: string)
    local h = mod(x)
    local res: string = h.response()
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected error for mod(x: string).response() via default optional return")
	}
}

// A literal dispatch may capture both a local table and a require alias. The
// table's empty registry makes its guarded return dead for the "http" case.
func TestLitretLiteralDispatchWithCapturedModuleRegistry(t *testing.T) {
	http := testutil.CheckAndExport(`
local http = {}
function http.response(): string return "ok" end
return http
`, "http", testutil.WithStdlib())
	if http.HasError() {
		t.Fatalf("http module errors: %v", testutil.ErrorMessages(http.Errors))
	}
	source := `
local http = require("http")
local M = { _modules = {} }
local function mod(name)
    if type(M._modules) == "table" and M._modules[name] ~= nil then
        return M._modules[name]
    end
    if name == "http" then return http end
    return nil
end
local h = mod("http")
local response: string = h.response()
return response
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("http", http))
	if result.HasError() {
		t.Fatalf("captured literal dispatch errors: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestLitretLiteralDispatchSkipsGeneralRegistryReturn(t *testing.T) {
	source := `
local M = { _modules = {} }
local http = { response = function(): string return "ok" end }
local function mod(name)
    if name == "http" then return http end
    return M._modules[name]
end
local h = mod("http")
local response: string = h.response()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("literal return should be selected: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestLitretOverloadKeepsErrorReturnCorrelation(t *testing.T) {
	source := `
local function lookup(text: string?)
    if not text or text == "" then return nil, "missing" end
    return text, nil
end
local function use(text: string?)
    local value, err = lookup(text)
    if err then return end
    local required: string = value
    return required
end
return use("ok")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("error guard should narrow the value: %v", testutil.ErrorMessages(result.Errors))
	}
}
