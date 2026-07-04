package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

// A module that installs ambient globals by assigning to _G must publish them,
// and an entry that requires the module must then recognize a bare reference
// instead of reporting an unknown value. This is the DSL pattern used by the
// test runner (describe/it) and the migration DSL (up/down).
func TestCheckModuleProvidedGlobalsRecognizedByImporter(t *testing.T) {
	mod := CheckAndExport(`
local M = {}
function M.describe(name, body) end
function M.it(name, body) end
function M.run_cases(fn)
    _G.describe = M.describe
    _G.it = M.it
    fn()
end
return M
`, "test")

	has := func(name string) bool {
		for _, g := range mod.Manifest.Globals {
			if g == name {
				return true
			}
		}
		return false
	}
	if !has("describe") || !has("it") {
		t.Fatalf("module did not publish provided globals: %v", mod.Manifest.Globals)
	}

	result := Check(`
local function define_tests()
    describe("suite", function()
        it("case", function()
            local x = 1
        end)
    end)
end
return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", mod.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want describe/it recognized via module-provided globals", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalTypesAnalyzeCallbackImporter(t *testing.T) {
	mod := CheckAndExport(`
local M = {}
function M.it(name: string, body: () -> ()): ()
    body()
end
function M.run_cases(fn: () -> ()): ()
    _G.it = M.it
    fn()
end
return M
`, "test", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want clean provider", mod.Errors)
	}
	if mod.Manifest.GlobalTypes["it"] == nil {
		t.Fatalf("manifest global types = %#v, want typed it ambient global", mod.Manifest.GlobalTypes)
	}

	result := Check(`
local function define_tests()
    it("case", function()
        local out: string = 1
    end)
end
return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", mod.Manifest))
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeAssignmentType) {
		t.Fatalf("diagnostics = %#v, want callback body analyzed through typed ambient global", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalTypesAnalyzeCapturedOptionalLocalInCallback(t *testing.T) {
	mod := CheckAndExport(`
local M = {}
function M.it(name: string, body: () -> ()): ()
    body()
end
function M.run_cases(fn: () -> ()): ()
    _G.it = M.it
    fn()
end
return M
`, "test", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want clean provider", mod.Errors)
	}

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local function define_tests()
    local lifecycle_runtime: Runtime?
    it("case", function()
        local out: string = lifecycle_runtime.apply("start")
    end)
end
return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", mod.Manifest))
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeOptionalMethodCall) {
		t.Fatalf("diagnostics = %#v, want captured optional local member access diagnosed", result.Diagnostics)
	}
}

func TestCheckUninitializedOptionalLocalMemberAccessDiagnosed(t *testing.T) {
	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local lifecycle_runtime: Runtime?
local out: string = lifecycle_runtime.apply("start")
`, WithStdlib())
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeOptionalMethodCall) {
		t.Fatalf("diagnostics = %#v, want optional local member access diagnosed", result.Diagnostics)
	}
}

// A wrapper that exposes a callback API backed by another module's ambient
// globals must republish those globals. This is the migration shape:
// migration.define(fn) returns a closure that eventually calls core.define(fn),
// and core.define installs database/up/down before invoking fn.
func TestCheckModuleProvidedGlobalsForwardThroughExportedCallbackWrapper(t *testing.T) {
	provider := CheckAndExport(`
local M = {}
function M.define(fn: () -> ()): ()
    _G.database = function(name: string, body: () -> ()) body() end
    _G.up = function(body: () -> ()) body() end
    fn()
end
return M
`, "provider", WithStdlib())

	wrapper := CheckAndExport(`
local M = {}
local provider = require("provider")
function M.define(fn: () -> ()): () -> ()
    return function()
        provider.define(fn)
    end
end
return M
`, "wrapper", WithStdlib(), WithManifest("provider", provider.Manifest))

	has := func(name string) bool {
		for _, g := range wrapper.Manifest.Globals {
			if g == name {
				return true
			}
		}
		return false
	}
	if !has("database") || !has("up") {
		t.Fatalf("wrapper did not republish forwarded provider globals: %v", wrapper.Manifest.Globals)
	}

	result := Check(`
local migration = require("wrapper")
return migration.define(function()
    database("sqlite", function()
        up(function()
            local ok: boolean = true
        end)
    end)
end)
`, WithStdlib(), WithManifest("wrapper", wrapper.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want wrapper-provided globals recognized in callback", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalsForwardThroughReturnedClosureAndInternalHelper(t *testing.T) {
	provider := CheckAndExport(`
local M = {}
function M.define(fn: () -> ()): ()
    _G.database = function(name: string, body: () -> ()) body() end
    _G.up = function(body: () -> ()) body() end
    _G.down = function(body: () -> ()) body() end
    fn()
end
return M
`, "provider", WithStdlib())

	wrapper := CheckAndExport(`
local M = {}
local provider = require("provider")

local function run(fn: () -> ()): ()
    pcall(provider.define, fn)
end

function M.define(fn: () -> ()): () -> ()
    return function()
        run(fn)
    end
end

return M
`, "wrapper", WithStdlib(), WithManifest("provider", provider.Manifest))

	has := func(name string) bool {
		for _, g := range wrapper.Manifest.Globals {
			if g == name {
				return true
			}
		}
		return false
	}
	if !has("database") || !has("up") || !has("down") {
		t.Fatalf("wrapper did not republish helper-forwarded provider globals: %v", wrapper.Manifest.Globals)
	}

	result := Check(`
return require("wrapper").define(function()
    database("sqlite", function()
        up(function()
            local ok: boolean = true
        end)
        down(function()
            local ok: boolean = true
        end)
    end)
end)
`, WithStdlib(), WithManifest("wrapper", wrapper.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want helper-forwarded wrapper globals recognized in callback", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalsDoNotLeakThroughUnusedProviderImport(t *testing.T) {
	provider := CheckAndExport(`
local M = {}
function M.define(fn: () -> ()): ()
    _G.database = function(name: string, body: () -> ()) body() end
    fn()
end
return M
`, "provider", WithStdlib())

	wrapper := CheckAndExport(`
local M = {}
local provider = require("provider")
function M.define(fn: () -> ()): () -> ()
    return function()
        local ignored = provider
        fn()
    end
end
return M
`, "wrapper", WithStdlib(), WithManifest("provider", provider.Manifest))

	for _, g := range wrapper.Manifest.Globals {
		if g == "database" {
			t.Fatalf("wrapper leaked provider global without forwarding callback: %v", wrapper.Manifest.Globals)
		}
	}
}

func TestCheckModuleProvidedGlobalsDoNotLeakThroughNonCallbackForward(t *testing.T) {
	provider := CheckAndExport(`
local M = {}
function M.define(value: any): ()
    _G.database = function(name: string, body: () -> ()) body() end
end
return M
`, "provider", WithStdlib())

	wrapper := CheckAndExport(`
local M = {}
local provider = require("provider")
function M.define(name: string): ()
    provider.define(name)
end
return M
`, "wrapper", WithStdlib(), WithManifest("provider", provider.Manifest))

	for _, g := range wrapper.Manifest.Globals {
		if g == "database" {
			t.Fatalf("wrapper leaked provider global through non-callback parameter: %v", wrapper.Manifest.Globals)
		}
	}
}

func TestCheckModuleProvidedGlobalsDoNotLeakThroughProtectedNonCallbackCall(t *testing.T) {
	provider := CheckAndExport(`
local M = {}
function M.define(fn: () -> ()): ()
    _G.database = function(name: string, body: () -> ()) body() end
    fn()
end
return M
`, "provider", WithStdlib())

	wrapper := CheckAndExport(`
local M = {}
local provider = require("provider")
function M.define(fn: () -> ()): () -> ()
    return function()
        pcall(provider.define, function() end)
        fn()
    end
end
return M
`, "wrapper", WithStdlib(), WithManifest("provider", provider.Manifest))

	for _, g := range wrapper.Manifest.Globals {
		if g == "database" {
			t.Fatalf("wrapper leaked provider global through protected non-callback call: %v", wrapper.Manifest.Globals)
		}
	}
}

func TestCheckModuleProvidedGlobalLifecycleSetupSeedsCaseCallback(t *testing.T) {
	testmod := lifecyclePhaseTestModule(t)

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local function define_tests()
    local lifecycle_runtime: Runtime?

    before_each(function()
        lifecycle_runtime = {
            apply = function(phase: string): string
                return phase
            end,
        }
    end)

    after_each(function()
        lifecycle_runtime = nil
    end)

    it("case", function()
        local out: string = lifecycle_runtime.apply("start")
    end)
end

return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want setup callback to seed case callback without after_each poisoning it", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalLifecyclePhasesInferredFromProviderReplay(t *testing.T) {
	testmod := CheckAndExport(`
local M = {}
local context = {
    current = nil,
}

function M.describe(name, body)
    context.current = {
        before_each = nil,
        after_each = nil,
        tests = {},
    }
    body()
end

function M.before_each(body)
    if context.current ~= nil then
        context.current.before_each = body
    end
end

function M.after_each(body)
    if context.current ~= nil then
        context.current.after_each = body
    end
end

function M.it(name, body)
    if context.current ~= nil then
        table.insert(context.current.tests, {
            name = name,
            fn = body,
        })
    end
end

local function get_suite_ancestry(suite)
    return { suite }
end

local function run_case(suite, test_case)
    local ancestry = get_suite_ancestry(suite)
    for _, ancestor in ipairs(ancestry) do
        if ancestor.before_each then
            ancestor.before_each()
        end
    end
    local ok, err = pcall(test_case.fn)
    for i = #ancestry, 1, -1 do
        local ancestor = ancestry[i]
        if ancestor.after_each then
            ancestor.after_each()
        end
    end
end

function M.run_cases(fn)
    _G.describe = M.describe
    _G.before_each = M.before_each
    _G.after_each = M.after_each
    _G.it = M.it
    fn()
end

return M
`, "test", WithStdlib())
	if len(testmod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want none", testmod.Errors)
	}
	if !hasCallbackPhaseRegistration(testmod.Manifest, "before_each", "before_each") {
		t.Fatalf("callback phase registrations = %#v, want before_each phase inferred from provider replay", testmod.Manifest.CallbackPhaseRegistrations)
	}
	if !hasCallbackPhaseInvocation(testmod.Manifest, "it", "before_each") {
		t.Fatalf("callback phase invocations = %#v, want it body to run after before_each", testmod.Manifest.CallbackPhaseInvocations)
	}

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local function define_tests()
    describe("suite", function()
        local lifecycle_runtime: Runtime?

        before_each(function()
            lifecycle_runtime = {
                apply = function(phase: string): string
                    return phase
                end,
            }
        end)

        after_each(function()
            lifecycle_runtime = nil
        end)

        it("case", function()
            local out: string = lifecycle_runtime.apply("start")
        end)
    end)
end

return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inferred setup callback to seed case callback", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalLifecycleInferenceRequiresReplayProof(t *testing.T) {
	testmod := CheckAndExport(`
local M = {}
local context = {
    current = {
        before_each = nil,
        tests = {},
    },
}

function M.before_each(body)
    context.current.before_each = body
end

function M.it(name, body)
    table.insert(context.current.tests, {
        name = name,
        fn = body,
    })
end

local function run_case(suite, test_case)
    test_case.fn()
end

function M.run_cases(fn)
    _G.before_each = M.before_each
    _G.it = M.it
    fn()
end

return M
`, "test", WithStdlib())
	if len(testmod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want none", testmod.Errors)
	}
	if hasCallbackPhaseInvocation(testmod.Manifest, "it", "before_each") {
		t.Fatalf("callback phase invocations = %#v, did not expect setup phase without replay proof", testmod.Manifest.CallbackPhaseInvocations)
	}

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local function define_tests()
    local lifecycle_runtime: Runtime?

    before_each(function()
        lifecycle_runtime = {
            apply = function(phase: string): string
                return phase
            end,
        }
    end)

    it("case", function()
        local out: string = lifecycle_runtime.apply("start")
    end)
end

return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeOptionalMethodCall) {
		t.Fatalf("diagnostics = %#v, want optional receiver when setup storage has no replay proof", result.Diagnostics)
	}
}

func TestCheckInferredLifecyclePhasesApplyToImportedModuleMemberCalls(t *testing.T) {
	testmod := CheckAndExport(`
local M = {}
M.hooks = {
    before_each = nil,
}
M.pending = nil

function M.before_each(body)
    M.hooks.before_each = body
end

function M.it(name, body)
    M.pending = {
        fn = body,
    }
end

local function run_pending()
    if M.hooks.before_each then
        M.hooks.before_each()
    end
    pcall(M.pending.fn)
end

return M
`, "test", WithStdlib())
	if len(testmod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want none", testmod.Errors)
	}

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local test = require("test")
local lifecycle_runtime: Runtime?

test.before_each(function()
    lifecycle_runtime = {
        apply = function(phase: string): string
            return phase
        end,
    }
end)

test.it("case", function()
    local out: string = lifecycle_runtime.apply("start")
end)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inferred phases to apply to imported module member calls", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalLifecycleSetupMustDominateCaseCallback(t *testing.T) {
	testmod := lifecyclePhaseTestModule(t)

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local function define_tests()
    local lifecycle_runtime: Runtime?

    it("case", function()
        local out: string = lifecycle_runtime.apply("start")
    end)

    before_each(function()
        lifecycle_runtime = {
            apply = function(phase: string): string
                return phase
            end,
        }
    end)
end

return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeOptionalMethodCall) {
		t.Fatalf("diagnostics = %#v, want optional receiver when setup registration does not dominate case", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalLifecyclePhasesPreserveDeclaredOrder(t *testing.T) {
	testmod := orderedLifecyclePhaseTestModule(t)

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local function define_tests()
    local lifecycle_runtime: Runtime?

    clear_runtime(function()
        lifecycle_runtime = nil
    end)

    seed_runtime(function()
        lifecycle_runtime = {
            apply = function(phase: string): string
                return phase
            end,
        }
    end)

    it("case", function()
        local out: string = lifecycle_runtime.apply("start")
    end)
end

return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want declared phase order clear -> seed to make runtime present", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalLifecycleDescribeCallbackIsAnalyzed(t *testing.T) {
	testmod := lifecyclePhaseTestModule(t)

	result := Check(`
local function define_tests()
    describe("suite", function()
        it("bad case", function()
            local value: string = 42
        end)
    end)
end

return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if !hasDiagnosticCode(result.Diagnostics, diagnostics.CodeAssignmentType) {
		t.Fatalf("diagnostics = %#v, want nested describe/it callback body to be analyzed", result.Diagnostics)
	}
}

func TestCheckModuleProvidedGlobalLifecycleNestedDescribeSeedsCaseCallback(t *testing.T) {
	testmod := lifecyclePhaseTestModule(t)

	result := Check(`
type Runtime = {
    apply: (string) -> string,
}

local function define_tests()
    describe("suite", function()
        local lifecycle_runtime: Runtime?

        before_each(function()
            lifecycle_runtime = {
                apply = function(phase: string): string
                    return phase
                end,
            }
        end)

        after_each(function()
            lifecycle_runtime = nil
        end)

        it("case", function()
            local out: string = lifecycle_runtime.apply("start")
        end)
    end)
end

return require("test").run_cases(define_tests)
`, WithStdlib(), WithManifest("test", testmod.Manifest))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want setup inside describe to seed nested case callback", result.Diagnostics)
	}
}

func lifecyclePhaseTestModule(t *testing.T) *ModuleResult {
	t.Helper()
	testmod := CheckAndExport(`
local M = {}

function M.describe(name: string, body: () -> ()): () end
function M.before_each(body: () -> ()): () end
function M.after_each(body: () -> ()): () end
function M.it(name: string, body: () -> ()): () end

function M.run_cases(fn: () -> ()): ()
    _G.describe = M.describe
    _G.before_each = M.before_each
    _G.after_each = M.after_each
    _G.it = M.it
    fn()
end

return M
`, "test", WithStdlib())
	if len(testmod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want none", testmod.Errors)
	}
	testmod.Manifest.DefineCallbackPhaseRegistration("before_each", 0, "setup")
	testmod.Manifest.DefineCallbackPhaseRegistration("after_each", 0, "teardown")
	testmod.Manifest.DefineCallbackPhaseInvocation("describe", 1, nil, nil)
	testmod.Manifest.DefineCallbackPhaseInvocation("it", 1, []string{"setup"}, []string{"teardown"})
	return testmod
}

func hasCallbackPhaseRegistration(m *manifest.Manifest, fn, phase string) bool {
	if m == nil {
		return false
	}
	for _, registration := range m.CallbackPhaseRegistrations {
		if registration.Function == fn && registration.Phase == phase {
			return true
		}
	}
	return false
}

func hasCallbackPhaseInvocation(m *manifest.Manifest, fn, before string) bool {
	if m == nil {
		return false
	}
	for _, invocation := range m.CallbackPhaseInvocations {
		if invocation.Function != fn {
			continue
		}
		for _, phase := range invocation.Before {
			if phase == before {
				return true
			}
		}
	}
	return false
}

func orderedLifecyclePhaseTestModule(t *testing.T) *ModuleResult {
	t.Helper()
	testmod := CheckAndExport(`
local M = {}

function M.clear_runtime(body: () -> ()): () end
function M.seed_runtime(body: () -> ()): () end
function M.it(name: string, body: () -> ()): () end

function M.run_cases(fn: () -> ()): ()
    _G.clear_runtime = M.clear_runtime
    _G.seed_runtime = M.seed_runtime
    _G.it = M.it
    fn()
end

return M
`, "test", WithStdlib())
	if len(testmod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want none", testmod.Errors)
	}
	testmod.Manifest.DefineCallbackPhaseRegistration("clear_runtime", 0, "z_clear")
	testmod.Manifest.DefineCallbackPhaseRegistration("seed_runtime", 0, "a_seed")
	testmod.Manifest.DefineCallbackPhaseInvocation("it", 1, []string{"z_clear", "a_seed"}, nil)
	return testmod
}

// An entry that does not require the providing module still reports the DSL
// globals as unknown - the globals are scoped to importers, not ambient
// everywhere.
func TestCheckModuleProvidedGlobalsScopedToImporters(t *testing.T) {
	result := Check(`
describe("suite", function() end)
`, WithStdlib())
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected unknown-value for describe without importing a module that provides it")
	}
}

// Lua base globals are recognized without loading typed stdlib signatures: the
// names are always present in the environment. This pins the rectification that
// replaced the hardcoded ambient-global name switch with a principled source.
func TestCheckLuaBaseGlobalsRecognizedWithoutStdlib(t *testing.T) {
	result := Check(`
local function f(x: number): number
    if x < 0 then
        error("negative")
    end
    return x
end
return f
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want error recognized as a base global without stdlib", result.Diagnostics)
	}
}
