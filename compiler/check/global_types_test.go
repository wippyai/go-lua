package check

import (
	"testing"

	analysistest "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	typemanifest "github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCheckerUsesGlobalTypesAsTypedEntryValues(t *testing.T) {
	chunk, err := parse.ParseString(`
local start = time.now()
local elapsed: number = time.now():sub(start):milliseconds()
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "milliseconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(durationType).Build()},
	})
	timeModule := typ.NewRecord().
		Field("now", typ.Func().Returns(timeType).Build()).
		Build()

	checker := NewChecker(db.New(), Deps{
		GlobalTypes: map[string]typ.Type{"time": timeModule},
	})
	session := checker.CheckChunk(chunk, "test.lua")
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want typed GlobalTypes to seed facade entry values", session.Diagnostics)
	}
}

func TestCheckerUsesGlobalTypeStaticStringConstantInMethodArgument(t *testing.T) {
	chunk, err := parse.ParseString(`
local formatted: string = time.now():format(time.RFC3339)
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "format", Type: typ.Func().Param("self", typ.Self).Param("layout", typ.String).Returns(typ.String).Build()},
	})
	timeModule := typ.NewRecord().
		Field("now", typ.Func().Returns(timeType).Build()).
		Field("RFC3339", typ.String).
		Build()

	checker := NewChecker(db.New(), Deps{
		GlobalTypes: map[string]typ.Type{"time": timeModule},
	})
	session := checker.CheckChunk(chunk, "test.lua")
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want global type static string constant to satisfy method argument", session.Diagnostics)
	}
}

func TestCheckerUsesCurrentImportAliasAsTypedEntryGlobal(t *testing.T) {
	chunk, err := parse.ParseString(`
local assert = assert2
local value: number = assert.value()
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	assertManifest := typemanifest.New("app.lib:assert")
	assertManifest.SetExport(typ.NewRecord().
		Field("value", typ.Func().Returns(typ.Number).Build()).
		Build())
	database := db.New()
	database.Connect("assert2", assertManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"assert2": assertManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want current import alias to seed typed global", session.Diagnostics)
	}
}

func TestCheckerResolvesRequireThroughCurrentImportAlias(t *testing.T) {
	chunk, err := parse.ParseString(`
local consts = require("consts")

local function notify(topic: string): ()
end

notify(consts.topic.IMAGE_BUILD_STATUS)
notify(consts.topic.IMAGE_BUILD_LOG)
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	constsManifest := typemanifest.New("userspace.docker.lib:consts")
	constsManifest.SetExport(typ.NewRecord().
		Field("topic", typ.NewRecord().
			Field("IMAGE_BUILD_LOG", typ.String).
			Field("IMAGE_BUILD_STATUS", typ.String).
			Build()).
		Build())
	database := db.New()
	database.Connect("userspace.docker.lib:consts", constsManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"consts": constsManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want require(\"consts\") to resolve current entry import alias", session.Diagnostics)
	}
}

func TestCheckerDischargesWrapperObligationWithCurrentImportAlias(t *testing.T) {
	chunk, err := parse.ParseString(`
local consts = require("consts")
local helpers = require("helpers")

local function notify_root(root_pid, topic, payload)
	if root_pid then
		helpers.send_json(root_pid, topic, payload)
	end
end

notify_root("root", consts.topic.IMAGE_BUILD_STATUS, {})
notify_root("root", consts.topic.IMAGE_BUILD_LOG, {})
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	constsManifest := typemanifest.New("userspace.docker.lib:consts")
	constsManifest.SetExport(typ.NewRecord().
		Field("topic", typ.NewRecord().
			Field("IMAGE_BUILD_LOG", typ.String).
			Field("IMAGE_BUILD_STATUS", typ.String).
			Build()).
		Build())
	helpersManifest := typemanifest.New("userspace.docker.lib:helpers")
	helpersManifest.SetExport(typ.NewRecord().
		Field("send_json", typ.Func().
			Param("root_pid", typ.NewOptional(typ.String)).
			Param("topic", typ.String).
			Param("payload", typ.Any).
			Returns().
			Build()).
		Build())
	database := db.New()
	database.Connect("userspace.docker.lib:consts", constsManifest)
	database.Connect("userspace.docker.lib:helpers", helpersManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"consts":  constsManifest,
		"helpers": helpersManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported constants to discharge projected wrapper obligations", session.Diagnostics)
	}
}

func TestCheckerDischargesWrapperObligationWithInferredImportedHelperManifest(t *testing.T) {
	processManifest := typemanifest.New("process")
	processManifest.SetExport(typ.NewRecord().
		Field("send", typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Param("data", typ.String).
			Returns().
			Build()).
		Build())
	jsonManifest := typemanifest.New("json")
	jsonManifest.SetExport(typ.NewRecord().
		Field("encode", typ.Func().
			Param("data", typ.Any).
			Returns(typ.String).
			Build()).
		Build())
	constsManifest := typemanifest.New("userspace.docker.lib:consts")
	constsManifest.SetExport(typ.NewRecord().
		Field("topic", typ.NewRecord().
			Field("IMAGE_BUILD_LOG", typ.String).
			Field("IMAGE_BUILD_STATUS", typ.String).
			Build()).
		Build())

	database := db.New()
	database.Connect("process", processManifest)
	database.Connect("json", jsonManifest)
	database.Connect("userspace.docker.lib:consts", constsManifest)
	checker := NewChecker(database, Deps{})

	helperChunk, err := parse.ParseString(`
local json = require("json")
local helpers = {}

function helpers.send_json(pid, topic, data)
	process.send(pid, topic, json.encode(data))
end

return helpers
`, "userspace.docker.lib:helpers")
	if err != nil {
		t.Fatalf("parse helpers: %v", err)
	}
	helperSession := checker.CheckChunkWithImports(helperChunk, "userspace.docker.lib:helpers", map[string]*typemanifest.Manifest{
		"json":    jsonManifest,
		"process": processManifest,
	})
	if len(helperSession.Diagnostics) != 0 {
		t.Fatalf("helper diagnostics = %#v, want inferred helper clean", helperSession.Diagnostics)
	}
	helpersManifest := helperSession.ExportManifest("userspace.docker.lib:helpers")
	helperSession.Release()

	chunk, err := parse.ParseString(`
local consts = require("consts")
local helpers = require("helpers")

local function notify_root(root_pid, topic, payload)
	if root_pid then
		helpers.send_json(root_pid, topic, payload)
	end
end

notify_root("root", consts.topic.IMAGE_BUILD_STATUS, {})
notify_root("root", consts.topic.IMAGE_BUILD_LOG, {})
`, "userspace.docker.service:image_builder")
	if err != nil {
		t.Fatalf("parse image builder: %v", err)
	}
	session := checker.CheckChunkWithImports(chunk, "userspace.docker.service:image_builder", map[string]*typemanifest.Manifest{
		"consts":  constsManifest,
		"helpers": helpersManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inferred imported helper manifest to preserve topic obligation", session.Diagnostics)
	}
}

func TestCheckerExportsStaticAssignedConstantTableForCurrentImportAlias(t *testing.T) {
	processManifest := typemanifest.New("process")
	processManifest.SetExport(typ.NewRecord().
		Field("send", typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Param("data", typ.String).
			Returns().
			Build()).
		Build())
	jsonManifest := typemanifest.New("json")
	jsonManifest.SetExport(typ.NewRecord().
		Field("encode", typ.Func().
			Param("data", typ.Any).
			Returns(typ.String).
			Build()).
		Build())

	database := db.New()
	database.Connect("process", processManifest)
	database.Connect("json", jsonManifest)
	checker := NewChecker(database, Deps{})

	constsChunk, err := parse.ParseString(`
local consts = {}

consts.topic = {
	IMAGE_BUILD_NEW = "image.build.new",
	IMAGE_BUILD_LOG = "image.build.log",
	IMAGE_BUILD_STATUS = "image.build.status",
}

consts.build_status = {
	FAILED = "failed",
}

return consts
`, "userspace.docker.lib:consts")
	if err != nil {
		t.Fatalf("parse consts: %v", err)
	}
	constsSession := checker.CheckChunk(constsChunk, "userspace.docker.lib:consts")
	if len(constsSession.Diagnostics) != 0 {
		t.Fatalf("consts diagnostics = %#v, want const table clean", constsSession.Diagnostics)
	}
	constsManifest := constsSession.ExportManifest("userspace.docker.lib:consts")
	constsSession.Release()
	database.Connect("userspace.docker.lib:consts", constsManifest)

	helperChunk, err := parse.ParseString(`
local json = require("json")
local helpers = {}

function helpers.send_json(pid, topic, data)
	process.send(pid, topic, json.encode(data))
end

return helpers
`, "userspace.docker.lib:helpers")
	if err != nil {
		t.Fatalf("parse helpers: %v", err)
	}
	helperSession := checker.CheckChunkWithImports(helperChunk, "userspace.docker.lib:helpers", map[string]*typemanifest.Manifest{
		"json":    jsonManifest,
		"process": processManifest,
	})
	if len(helperSession.Diagnostics) != 0 {
		t.Fatalf("helper diagnostics = %#v, want inferred helper clean", helperSession.Diagnostics)
	}
	helpersManifest := helperSession.ExportManifest("userspace.docker.lib:helpers")
	helperSession.Release()

	chunk, err := parse.ParseString(`
local consts = require("consts")
local helpers = require("helpers")

local function notify_root(root_pid, topic, payload)
	if root_pid then
		helpers.send_json(root_pid, topic, payload)
	end
end

notify_root("root", consts.topic.IMAGE_BUILD_STATUS, {})
notify_root("root", consts.topic.IMAGE_BUILD_LOG, {})
`, "userspace.docker.service:image_builder")
	if err != nil {
		t.Fatalf("parse image builder: %v", err)
	}
	session := checker.CheckChunkWithImports(chunk, "userspace.docker.service:image_builder", map[string]*typemanifest.Manifest{
		"consts":  constsManifest,
		"helpers": helpersManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want source-derived const table to satisfy wrapper topic obligation", session.Diagnostics)
	}
}

func TestCheckerKeepsImportedConstantPrecisionInsideNestedFunction(t *testing.T) {
	processManifest := typemanifest.New("process")
	processManifest.SetExport(typ.NewRecord().
		Field("send", typ.Func().
			Param("pid", typ.Any).
			Param("topic", typ.String).
			Param("data", typ.String).
			Returns().
			Build()).
		Build())
	jsonManifest := typemanifest.New("json")
	jsonManifest.SetExport(typ.NewRecord().
		Field("encode", typ.Func().
			Param("data", typ.Any).
			Returns(typ.String).
			Build()).
		Build())
	imagesRepoManifest := typemanifest.New("userspace.docker.persist:images")
	imagesRepoManifest.SetExport(typ.NewRecord().
		Field("get", typ.Func().
			Param("db", typ.Any).
			Param("id", typ.String).
			Returns(typ.Any).
			Build()).
		Field("update_build", typ.Func().
			Param("db", typ.Any).
			Param("id", typ.String).
			Param("status", typ.String).
			Param("updates", typ.Any).
			Returns().
			Build()).
		Build())
	emptyManifest := func(path string) *typemanifest.Manifest {
		manifest := typemanifest.New(path)
		manifest.SetExport(typ.Any)
		return manifest
	}
	sqlManifest := emptyManifest("sql")
	timeManifest := emptyManifest("time")
	tarManifest := emptyManifest("userspace.docker.lib:tar")
	dockerClientManifest := emptyManifest("userspace.docker:docker_client")
	loggerManifest := emptyManifest("logger")
	database := db.New()
	database.Connect("process", processManifest)
	database.Connect("json", jsonManifest)
	database.Connect("userspace.docker.persist:images", imagesRepoManifest)
	database.Connect("sql", sqlManifest)
	database.Connect("time", timeManifest)
	database.Connect("userspace.docker.lib:tar", tarManifest)
	database.Connect("userspace.docker:docker_client", dockerClientManifest)
	database.Connect("logger", loggerManifest)
	checker := NewChecker(database, Deps{})

	constsChunk, err := parse.ParseString(`
local consts = {}

consts.topic = {
	IMAGE_BUILD_LOG = "image.build.log",
	IMAGE_BUILD_STATUS = "image.build.status",
}

consts.build_status = {
	FAILED = "failed",
}

return consts
`, "userspace.docker.lib:consts")
	if err != nil {
		t.Fatalf("parse consts: %v", err)
	}
	constsSession := checker.CheckChunk(constsChunk, "userspace.docker.lib:consts")
	if len(constsSession.Diagnostics) != 0 {
		t.Fatalf("consts diagnostics = %#v, want const table clean", constsSession.Diagnostics)
	}
	constsManifest := constsSession.ExportManifest("userspace.docker.lib:consts")
	constsSession.Release()
	database.Connect("userspace.docker.lib:consts", constsManifest)

	helpersChunk, err := parse.ParseString(`
local json = require("json")
local helpers = {}

function helpers.send_json(pid, topic, data)
	process.send(pid, topic, json.encode(data))
end

return helpers
`, "userspace.docker.lib:helpers")
	if err != nil {
		t.Fatalf("parse helpers: %v", err)
	}
	helpersSession := checker.CheckChunkWithImports(helpersChunk, "userspace.docker.lib:helpers", map[string]*typemanifest.Manifest{
		"json":    jsonManifest,
		"process": processManifest,
	})
	if len(helpersSession.Diagnostics) != 0 {
		t.Fatalf("helper diagnostics = %#v, want helper clean", helpersSession.Diagnostics)
	}
	helpersManifest := helpersSession.ExportManifest("userspace.docker.lib:helpers")
	helpersSession.Release()
	database.Connect("userspace.docker.lib:helpers", helpersManifest)

	chunk, err := parse.ParseString(`
local sql = require("sql")
local json = require("json")
local time = require("time")
local consts = require("consts")
local images_repo = require("images_repo")
local tar = require("tar")
local docker_client = require("docker_client")
local helpers = require("helpers")
local logger = require("logger")

local function notify_root(root_pid, topic, payload)
	if root_pid then
		helpers.send_json(root_pid, topic, payload)
	end
end

local function run_build(docker: any, root_pid)
	local image = images_repo.get(nil, "image-1")
	if not image then
		images_repo.update_build(nil, "build-1", consts.build_status.FAILED, {
			error = "image record not found",
		})
		notify_root(root_pid, consts.topic.IMAGE_BUILD_STATUS, {})
		notify_root(root_pid, consts.topic.IMAGE_BUILD_LOG, {})
		return
	end
	local lines, build_err = docker:build_image("", "", "")
end

local image_builder = {}

function image_builder.run(root_pid, topic)
	if topic == consts.topic.IMAGE_BUILD_NEW then
		coroutine.spawn(function()
			run_build({}, root_pid)
		end)
	end
end

return image_builder
`, "userspace.docker.service:image_builder")
	if err != nil {
		t.Fatalf("parse image builder: %v", err)
	}
	session := checker.CheckChunkWithImports(chunk, "userspace.docker.service:image_builder", map[string]*typemanifest.Manifest{
		"consts":        constsManifest,
		"docker_client": dockerClientManifest,
		"helpers":       helpersManifest,
		"images_repo":   imagesRepoManifest,
		"json":          jsonManifest,
		"logger":        loggerManifest,
		"sql":           sqlManifest,
		"tar":           tarManifest,
		"time":          timeManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nested function to keep imported constant precision", session.Diagnostics)
	}
}

func TestCheckerPreservesAnnotatedArrayReturnThroughFacade(t *testing.T) {
	chunk, err := parse.ParseString(`
local function run_suite(name: string, tests: {any})
    return 0, {}
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        return true
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}
    for _, entry in ipairs(entries) do
        table.insert(no_suite, entry)
    end
    sort_tests(no_suite)
    return suites, no_suite
end

local function run(entries)
    local suites, no_suite = group_by_suite(entries)
    if #no_suite > 0 then
        local _, failures = run_suite("other", no_suite)
    end
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	checker := NewChecker(db.New(), Deps{})
	session := checker.CheckChunk(chunk, "test.lua")
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want facade to preserve annotated array return", session.Diagnostics)
	}
}

func TestCheckerPreservesAnnotatedArrayReturnThroughPCallFacade(t *testing.T) {
	chunk, err := parse.ParseString(`
local function run_suite(name: string, tests: {any})
    return 0, {}
end

local function sort_tests(tests)
    table.sort(tests, function(a, b)
        return true
    end)
    return tests
end

local function group_by_suite(entries)
    local suites: {[string]: any[]} = {}
    local no_suite: any[] = {}
    for _, entry in ipairs(entries) do
        table.insert(no_suite, entry)
    end
    sort_tests(no_suite)
    return suites, no_suite
end

local function run_tests()
    local suites, no_suite = group_by_suite({})
    if #no_suite > 0 then
        local _, failures = run_suite("other", no_suite)
    end
    return 0
end

local function main()
    local ok, result = pcall(run_tests)
    if not ok then
        return 1
    end
    return result
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	checker := NewChecker(db.New(), Deps{})
	session := checker.CheckChunk(chunk, "test.lua")
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want facade to preserve annotated array return through pcall wrapper", session.Diagnostics)
	}
}

func TestCheckerDoesNotLeakDBImportsAsEntryGlobals(t *testing.T) {
	chunk, err := parse.ParseString(`
local assert = assert2
local value: number = assert.value()
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	assertManifest := typemanifest.New("app.lib:assert")
	assertManifest.SetExport(typ.NewRecord().
		Field("value", typ.Func().Returns(typ.Number).Build()).
		Build())
	database := db.New()
	database.Connect("assert2", assertManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunk(chunk, "test.lua")
	if len(session.Diagnostics) == 0 {
		t.Fatal("expected unknown-value diagnostic for import alias that was not provided as a current entry import")
	}
}

func TestCheckerPreservesImportAliasSignatureEffectsThroughLocalAlias(t *testing.T) {
	chunk, err := parse.ParseString(`
local assert = assert2

type AppError = {
	kind: fun(self): string,
}

local err: AppError? = {
	kind = function(self): string
		return "invalid"
	end,
}

assert.has_error(nil, err)
local kind: string = err:kind()
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	assertManifest := typemanifest.New("app.lib:assert")
	assertManifest.SetExport(typ.NewRecord().
		Field("has_error", typ.Func().
			Param("value", typ.Any).
			Param("err", typ.Any).
			Returns().
			Build()).
		Build())
	assertManifest.DefineFunctionSignature("app.lib:assert.has_error", signature.Function{
		Type: typ.Func().
			Param("value", typ.Any).
			Param("err", typ.Any).
			Returns().
			Build(),
		OperationalEffects: &signature.OperationalEffects{
			NormalReturnPresenceRefinements: []signature.PathPresenceRefinement{{
				Path:     path.NewPlaceholder(1),
				Presence: presence.Present(),
			}},
		},
	})
	database := db.New()
	database.Connect("assert2", assertManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"assert2": assertManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local alias to preserve import signature effects", session.Diagnostics)
	}
}

func TestCheckerPreservesImportAliasSignatureEffectsThroughRequire(t *testing.T) {
	assertMod := analysistest.CheckAndExport(`
local M = {}

function M.is_string(value, msg)
	if type(value) ~= "string" then
		error(msg or "expected string", 2)
	end
	return value
end

return M
`, "app.lib:assert", analysistest.WithStdlib())
	if len(assertMod.Errors) != 0 {
		t.Fatalf("assert module diagnostics = %#v, want clean helper export", assertMod.Errors)
	}
	assertManifest := roundTripFacadeManifest(t, "app.lib:assert", assertMod.Manifest)

	chunk, err := parse.ParseString(`
local assert = require("assert2")

local function check(result: {err: any}): boolean
	assert.is_string(result.err, "error must be string, got " .. type(result.err))
	local hit = string.find(result.err, "not allowed", 1, true)
	return hit ~= nil
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	database := db.New()
	database.Connect("assert2", assertManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"assert2": assertManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want require alias to preserve import signature effects", session.Diagnostics)
	}
}

func TestCheckerPreservesRequireAliasSignatureEffectsFromDirtyModuleManifest(t *testing.T) {
	assertMod := analysistest.CheckAndExport(`
local M = {}

function M.is_string(value, msg)
	if type(value) ~= "string" then
		error(msg or "expected string", 2)
	end
	return value
end

function M.not_nil(value, msg)
	if value == nil then
		error(msg or "expected non-nil", 2)
	end
	return value
end

function M.unrelated_bad_helper(err, substr)
	local actual_msg = type(err) == "table" and err.message or tostring(err)
	return string.find(actual_msg, substr, 1, true)
end

return M
`, "app.lib:assert", analysistest.WithStdlib())
	if len(assertMod.Errors) == 0 {
		t.Fatalf("assert module unexpectedly clean; fixture must keep an unrelated dirty helper")
	}
	assertManifest := roundTripFacadeManifest(t, "app.lib:assert", assertMod.Manifest)

	chunk, err := parse.ParseString(`
local assert = require("assert2")

local function check(result: {err: any}): boolean
	assert.is_string(result.err, "error must be string, got " .. type(result.err))
	local hit = string.find(result.err, "not allowed", 1, true)
	return hit ~= nil
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	database := db.New()
	database.Connect("assert2", assertManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"assert2": assertManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want dirty imported module to preserve independent require alias signature effects", session.Diagnostics)
	}
}

func TestCheckerPreservesRequireAliasSignatureEffectsOnAnyRoot(t *testing.T) {
	assertMod := analysistest.CheckAndExport(`
local M = {}

function M.is_string(value, msg)
	if type(value) ~= "string" then
		error(msg or "expected string", 2)
	end
	return value
end

function M.unrelated_bad_helper(err, substr)
	local actual_msg = type(err) == "table" and err.message or tostring(err)
	return string.find(actual_msg, substr, 1, true)
end

return M
`, "app.lib:assert", analysistest.WithStdlib())
	if len(assertMod.Errors) == 0 {
		t.Fatalf("assert module unexpectedly clean; fixture must keep an unrelated dirty helper")
	}
	assertManifest := roundTripFacadeManifest(t, "app.lib:assert", assertMod.Manifest)

	chunk, err := parse.ParseString(`
local assert = require("assert2")

local function check(result: any): boolean
	assert.not_nil(result, "result required")
	assert.is_string(result.err, "error must be string, got " .. type(result.err))
	local hit = string.find(result.err, "not allowed", 1, true)
		or string.find(result.err, "network", 1, true)
	return hit ~= nil
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	database := db.New()
	database.Connect("assert2", assertManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"assert2": assertManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want dirty imported module to refine any-root member path through require alias signature effects", session.Diagnostics)
	}
}

func TestCheckerPreservesDirtyImportedBuilderErrorReturnCorrelation(t *testing.T) {
	promptMod := analysistest.CheckAndExport(`
local M = {}

type Message = { role: string, content: string? }
type Builder = {
	get_messages: (self: table) -> {Message},
}

function M.new(messages: {Message}?)
	local builder = {
		messages = messages or {},
	}
	builder.get_messages = function(self: any)
		return self.messages
	end
	return builder
end

return M
`, "prompt", analysistest.WithStdlib())
	if len(promptMod.Errors) != 0 {
		t.Fatalf("prompt diagnostics = %#v, want clean constructor export", promptMod.Errors)
	}
	promptNewSig, ok := promptMod.Manifest.FunctionSignatures["prompt.new"]
	if !ok {
		t.Fatalf("missing prompt.new signature: %#v", promptMod.Manifest.FunctionSignatures)
	}
	if promptNewSig.Type == nil || len(promptNewSig.Type.Returns) == 0 {
		t.Fatalf("prompt.new signature = %#v, want inferred constructor return type", promptNewSig)
	}

	builderMod := analysistest.CheckAndExport(`
local M = {
	_prompt = require("prompt"),
}

function M.build(messages)
	if not messages then
		return nil, "Messages are required"
	end

	local builder = M._prompt.new()
	for _, msg in ipairs(messages) do
		local metadata: table = msg.metadata or {}
		if metadata.skip then
			builder:get_messages()
		end
	end

	return builder, nil
end

return M
`, "prompt_builder", analysistest.WithStdlib(), analysistest.WithModule("prompt", promptMod))
	if len(builderMod.Errors) == 0 {
		t.Fatalf("builder diagnostics = %#v, want dirty body to exercise exported partial summary", builderMod.Errors)
	}
	builderManifest := roundTripFacadeManifest(t, "prompt_builder", builderMod.Manifest)
	buildSig, ok := builderManifest.FunctionSignatures["prompt_builder.build"]
	if !ok {
		t.Fatalf("missing prompt_builder.build signature: %#v", builderManifest.FunctionSignatures)
	}
	if buildSig.Effect.Pure() {
		t.Fatalf("prompt_builder.build effect = %s, want exported error-return effect", buildSig.Effect)
	}
	if buildSig.OperationalEffects == nil || len(buildSig.OperationalEffects.ReturnPresenceRelations) == 0 {
		t.Fatalf("prompt_builder.build operational return relations = %#v, want exported error-return presence relation", buildSig.OperationalEffects)
	}

	assertMod := analysistest.CheckAndExport(`
local M = {}

function M.is_nil(value: any, msg: string?)
	if value ~= nil then
		error(msg or "expected nil")
	end
end

return M
`, "testassert", analysistest.WithStdlib())
	if len(assertMod.Errors) != 0 {
		t.Fatalf("assert diagnostics = %#v, want clean helper export", assertMod.Errors)
	}
	assertManifest := roundTripFacadeManifest(t, "testassert", assertMod.Manifest)

	chunk, err := parse.ParseString(`
local prompt_builder = require("prompt_builder")
local test = require("testassert")

local builder, err = prompt_builder.build({
	{ metadata = { skip = false } },
})
test.is_nil(err)
local messages = builder:get_messages()
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	database := db.New()
	database.Connect("prompt_builder", builderManifest)
	database.Connect("testassert", assertManifest)

	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"prompt_builder": builderManifest,
		"testassert":     assertManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want dirty imported build result relation plus nil assertion to prove builder present", session.Diagnostics)
	}
}

func TestCheckerPreservesManifestAliasCastThroughUnannotatedChannelSelectHelper(t *testing.T) {
	chunk, err := parse.ParseString(`
type Message = process.Message
type MessageChannel = Channel<Message>

local function payload_data(msg: Message): any?
	local p = msg:payload()
	return p and p:data() or nil
end

local function wait_for_topic(inbox: MessageChannel, deadline: unknown)
	while true do
		local result = channel.select {
			inbox:case_receive(),
			deadline:case_receive(),
		}
		if result.channel == deadline then
			return nil, "timeout waiting for message"
		end
		local msg = result.value as Message
		if msg:topic() == "ack" then
			return msg, nil
		end
	end
end

local function main(deadline: unknown): boolean
	local inbox = process.inbox() as MessageChannel
	local msg, wait_err = wait_for_topic(inbox, deadline)
	if wait_err then
		return false
	end
	if msg == nil then
		error("missing message")
	end
	local data = payload_data(msg as Message)
	return data ~= nil
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	channelManifest := typemanifest.New("channel")
	channelManifest.DefineType("Channel", ambient.ChannelGeneric())
	channelManifest.SetExport(typ.Any)

	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})
	messageChannelType := typ.Instantiate(ambient.ChannelGeneric(), messageType)
	processManifest := typemanifest.New("process")
	processManifest.DefineType("Message", messageType)
	processManifest.SetExport(typ.NewRecord().
		Field("inbox", typ.Func().Returns(messageChannelType).Build()).
		Build())

	database := db.New()
	database.Connect("channel", channelManifest)
	database.Connect("process", processManifest)

	checker := NewChecker(database, Deps{
		GlobalTypes: map[string]typ.Type{
			"channel": typ.Any,
			"process": processManifest.Export,
		},
	})
	session := checker.CheckChunk(chunk, "test.lua")
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want manifest alias cast to survive facade checker path", session.Diagnostics)
	}
}

func TestCheckerManifestAliasCastValidatesAnyReturningCall(t *testing.T) {
	chunk, err := parse.ParseString(`
type Message = process.Message
type MessageChannel = Channel<Message>

local function wait_for_topic(inbox: MessageChannel): ()
	local case = inbox:case_receive()
end

local function main(): ()
	local inbox = process.inbox() as MessageChannel
	wait_for_topic(inbox)
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	channelManifest := typemanifest.New("channel")
	channelManifest.DefineType("Channel", ambient.ChannelGeneric())
	channelManifest.SetExport(typ.Any)

	messageType := typ.NewInterface("process.Message", []typ.Method{
		{Name: "topic", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	processManifest := typemanifest.New("process")
	processManifest.DefineType("Message", messageType)
	processManifest.SetExport(typ.NewRecord().
		Field("inbox", typ.Func().Returns(typ.Any).Build()).
		Build())

	database := db.New()
	database.Connect("channel", channelManifest)
	database.Connect("process", processManifest)

	checker := NewChecker(database, Deps{
		GlobalTypes: map[string]typ.Type{
			"channel": typ.Any,
			"process": processManifest.Export,
		},
	})
	session := checker.CheckChunk(chunk, "test.lua")
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want concrete manifest alias cast to validate any-returning call", session.Diagnostics)
	}
}

func TestCheckerAmbientGlobalTypeWinsOverSameNamedImportAlias(t *testing.T) {
	provider := analysistest.CheckAndExport(`
local M = {}

function M.define(fn: () -> ()): ()
    _G.migration = function(name: string, body: () -> ()): () body() end
    fn()
end

return M
`, "migration", analysistest.WithStdlib())
	if len(provider.Errors) != 0 {
		t.Fatalf("provider diagnostics = %#v, want clean provider", provider.Errors)
	}
	migrationManifest := roundTripFacadeManifest(t, "migration", provider.Manifest)

	chunk, err := parse.ParseString(`
return require("migration").define(function()
    migration("create", function()
        local ok: boolean = true
    end)
end)
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	database := db.New()
	database.Connect("migration", migrationManifest)
	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"migration": migrationManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want provided ambient global to own same-named import alias in callback", session.Diagnostics)
	}
}

func TestCheckerPreservesImportedAssertionEffectsForMigrationDefineCallback(t *testing.T) {
	testMod := analysistest.CheckAndExport(`
local test = {}

function test.not_nil(value: any, msg: string?): any
    if value == nil then
        error(msg or "expected non-nil", 2)
    end
    return value
end

function test.is_function(value: any, msg: string?)
    if type(value) ~= "function" then
        error(msg or "expected function", 2)
    end
end

function test.describe(name, fn)
    fn()
end

function test.it(name, fn)
    fn()
end

function test.run_cases(fn)
    fn()
end

return test
`, "test", analysistest.WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want clean helper export", testMod.Errors)
	}

	coreMod := analysistest.CheckAndExport(`
type DatabaseImpl = {
    type: string?,
    up: any?,
    after: any?,
    down: any?,
}

type MigrationItem = {
    description: string,
    database_implementations: {[string]: DatabaseImpl},
}

type MigrationContext = {
    current_migration: MigrationItem?,
    current_database: DatabaseImpl?,
    implementations: {MigrationItem},
}

local core = {}

function core.create_context(): MigrationContext
    return {
        current_migration = nil,
        current_database = nil,
        implementations = {},
    } :: MigrationContext
end

function core.create_migration_fn(context: MigrationContext): any
    return function(description: string, fn: () -> ()): MigrationItem
        local old_migration = context.current_migration
        context.current_migration = {
            description = description,
            database_implementations = {},
        }
        local success, err = pcall(fn)
        if not success then
            context.current_migration = old_migration
            error("Error in migration definition: " .. tostring(err))
        end
        table.insert(context.implementations, context.current_migration)
        context.current_migration = old_migration
        return context.implementations[#context.implementations] :: MigrationItem
    end
end

function core.create_database_fn(context: MigrationContext): any
    return function(db_type: string, fn: () -> ()): ()
        if not context.current_migration then
            error("database() must be called within a migration block")
        end
        local old_database = context.current_database
        context.current_database = {
            type = db_type,
            up = nil,
            after = nil,
            down = nil,
        }
        local success, err = pcall(fn)
        if not success then
            context.current_database = old_database
            error("Error in database implementation: " .. tostring(err))
        end
        context.current_migration.database_implementations[db_type] = context.current_database
        context.current_database = old_database
    end
end

function core.create_up_fn(context: MigrationContext): any
    return function(fn: any): ()
        if not context.current_database then
            error("up() must be called within a database block")
        end
        if type(fn) ~= "function" then
            error("Up migration must be a function")
        end
        context.current_database.up = fn
    end
end

function core.create_after_fn(context: MigrationContext): any
    return function(fn: any): ()
        if not context.current_database then
            error("after() must be called within a database block")
        end
        if type(fn) ~= "function" then
            error("After hook must be a function")
        end
        context.current_database.after = fn
    end
end

function core.create_down_fn(context: MigrationContext): any
    return function(fn: any): ()
        if not context.current_database then
            error("down() must be called within a database block")
        end
        if type(fn) ~= "function" then
            error("Down migration must be a function")
        end
        context.current_database.down = fn
    end
end

function core.setup_globals(context: MigrationContext): ()
    _G.migration = core.create_migration_fn(context)
    _G.database = core.create_database_fn(context)
    _G.up = core.create_up_fn(context)
    _G.after = core.create_after_fn(context)
    _G.down = core.create_down_fn(context)
end

function core.cleanup_globals(): ()
    _G.migration = nil
    _G.database = nil
    _G.up = nil
    _G.after = nil
    _G.down = nil
end

function core.define(fn: () -> ()): {MigrationItem}
    if type(fn) ~= "function" then
        error("Migration definition must be a function")
    end
    local context = core.create_context()
    core.setup_globals(context)
    local success, err = pcall(fn)
    core.cleanup_globals()
    if not success then
        error("Error in migration definition: " .. tostring(err))
    end
    return context.implementations
end

function core.validate_implementation(implementation: MigrationItem, db_type: string): (boolean, string?)
    local impl = implementation.database_implementations[db_type]
    if not impl then
        return false, "No implementation for database type: " .. db_type
    end
    if not impl.up or type(impl.up) ~= "function" then
        return false, "Missing 'up' function for " .. db_type
    end
    if not impl.down or type(impl.down) ~= "function" then
        return false, "Missing 'down' function for " .. db_type
    end
    return true, nil
end

return core
`, "core", analysistest.WithStdlib())
	if len(coreMod.Errors) != 0 {
		t.Fatalf("core module diagnostics = %#v, want clean helper export", coreMod.Errors)
	}

	testManifest := roundTripFacadeManifest(t, "test", testMod.Manifest)
	coreManifest := roundTripFacadeManifest(t, "core", coreMod.Manifest)
	chunk, err := parse.ParseString(`
local core = require("core")
local test = require("test")

local function define_tests()
    test.describe("define", function()
        test.it("captures database-specific up/down/after", function()
            local implementations = core.define(function()
                migration("test migration", function()
                    database("sqlite", function()
                        up(function(db: any) end)
                        down(function(db: any) end)
                        after(function(db: any) end)
                    end)
                end)
            end)
            local impl = implementations[1].database_implementations["sqlite"]
            test.not_nil(impl)
            test.is_function(impl.up)
            test.is_function(impl.down)
            test.is_function(impl.after)

            impl.up(nil)
            impl.down(nil)
            impl.after(nil)
        end)
    end)
end

return {
    run_tests = function()
        return test.run_cases(define_tests)
    end
}
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	database := db.New()
	database.Connect("core", coreManifest)
	database.Connect("test", testManifest)
	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"core": coreManifest,
		"test": testManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want facade to preserve imported assertion effects for migration define callback", session.Diagnostics)
	}
}

func TestCheckerPreservesCallbackPhaseMetadataThroughFacade(t *testing.T) {
	testMod := analysistest.CheckAndExport(`
local test = {}

function test.describe(name: string, body: () -> ()): ()
    body()
end

function test.before_each(body: () -> ()): () end
function test.after_each(body: () -> ()): () end
function test.it(name: string, body: () -> ()): () end

function test.run_cases(fn: () -> ()): ()
    _G.describe = test.describe
    _G.before_each = test.before_each
    _G.after_each = test.after_each
    _G.it = test.it
    fn()
end

return test
`, "test", analysistest.WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want clean helper export", testMod.Errors)
	}
	testMod.Manifest.DefineCallbackPhaseRegistration("before_each", 0, "setup")
	testMod.Manifest.DefineCallbackPhaseRegistration("after_each", 0, "teardown")
	testMod.Manifest.DefineCallbackPhaseInvocation("it", 1, []string{"setup"}, []string{"teardown"})

	chunk, err := parse.ParseString(`
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
            local out: string = lifecycle_runtime.apply("activate")
        end)
    end)
end

return {
    run_tests = function()
        return require("test").run_cases(define_tests)
    end
}
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	testManifest := roundTripFacadeManifest(t, "test", testMod.Manifest)
	database := db.New()
	database.Connect("test", testManifest)
	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"test": testManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want facade to preserve callback phase setup for case body", session.Diagnostics)
	}
}

func TestCheckerRequiresCallbackPhaseMetadataForFrameworkSetupSemantics(t *testing.T) {
	testMod := analysistest.CheckAndExport(`
local test = {}

function test.describe(name: string, body: () -> ()): ()
    body()
end

function test.before_each(body: () -> ()): () end
function test.after_each(body: () -> ()): () end
function test.it(name: string, body: () -> ()): () end

function test.run_cases(fn: () -> ()): ()
    _G.describe = test.describe
    _G.before_each = test.before_each
    _G.after_each = test.after_each
    _G.it = test.it
    fn()
end

return test
`, "test", analysistest.WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module diagnostics = %#v, want clean helper export", testMod.Errors)
	}

	chunk, err := parse.ParseString(`
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

        it("case", function()
            local out: string = lifecycle_runtime.apply("activate")
        end)
    end)
end

return require("test").run_cases(define_tests)
`, "test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	testManifest := roundTripFacadeManifest(t, "test", testMod.Manifest)
	database := db.New()
	database.Connect("test", testManifest)
	checker := NewChecker(database, Deps{})
	session := checker.CheckChunkWithImports(chunk, "test.lua", map[string]*typemanifest.Manifest{
		"test": testManifest,
	})
	if len(session.Diagnostics) == 0 {
		t.Fatal("expected diagnostics without callback phase metadata")
	}
}

func TestCheckerFacadePreservesImportedSimpleLiteralArgumentReturnShape(t *testing.T) {
	mapperMod := analysistest.CheckAndExport(`
local M = {}

function M.map_tool_config(choice, available_tools)
    if not choice or choice == "auto" or choice == "any" or choice == "" then
        return { mode = "AUTO" }, nil
    elseif choice == "none" then
        return { mode = "NONE" }, nil
    elseif type(choice) == "string" then
        for _, tool in ipairs(available_tools or {}) do
            if tool.name == choice then
                return {
                    mode = "ANY",
                    allowedFunctionNames = { choice },
                }, nil
            end
        end
        return nil, "not found"
    end
    return "AUTO", nil
end

return M
`, "wippy.llm.google:mapper", analysistest.WithStdlib())
	if len(mapperMod.Errors) != 0 {
		t.Fatalf("mapper diagnostics = %#v, want clean export", mapperMod.Errors)
	}
	mapperManifest := roundTripFacadeManifest(t, "google_mapper", mapperMod.Manifest)

	chunk, err := parse.ParseString(`
local mapper = require("google_mapper")

local test_tools = {
    { name = "get_weather" },
    { name = "calculate" },
}

local auto_config, auto_error = mapper.map_tool_config("auto", test_tools)
local auto_mode: string = auto_config.mode

local none_config, none_error = mapper.map_tool_config("none", test_tools)
local none_mode: string = none_config.mode
`, "mapper_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	checker := NewChecker(db.New(), Deps{})
	session := checker.CheckChunkWithImports(chunk, "wippy.llm.google:mapper_test", map[string]*typemanifest.Manifest{
		"google_mapper": mapperManifest,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want facade import to preserve simple literal-argument return shape", session.Diagnostics)
	}
}

func TestCheckerFacadeKeepsImportedNamedLoopReturnShapeFrontier(t *testing.T) {
	mapperMod := analysistest.CheckAndExport(`
local M = {}

function M.map_tool_config(choice, available_tools)
    if not choice or choice == "auto" or choice == "any" or choice == "" then
        return { mode = "AUTO" }, nil
    elseif choice == "none" then
        return { mode = "NONE" }, nil
    elseif type(choice) == "string" then
        for _, tool in ipairs(available_tools or {}) do
            if tool.name == choice then
                return {
                    mode = "ANY",
                    allowedFunctionNames = { choice },
                }, nil
            end
        end
        return nil, "not found"
    end
    return "AUTO", nil
end

return M
`, "wippy.llm.google:mapper", analysistest.WithStdlib())
	if len(mapperMod.Errors) != 0 {
		t.Fatalf("mapper diagnostics = %#v, want clean export", mapperMod.Errors)
	}
	mapperManifest := roundTripFacadeManifest(t, "google_mapper", mapperMod.Manifest)

	chunk, err := parse.ParseString(`
local mapper = require("google_mapper")

local test_tools = {
    { name = "get_weather" },
    { name = "calculate" },
}

local named_config, named_error = mapper.map_tool_config("get_weather", test_tools)
local named_mode: string = named_config.mode
local named_allowed: string = named_config.allowedFunctionNames[1]
`, "mapper_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	checker := NewChecker(db.New(), Deps{})
	session := checker.CheckChunkWithImports(chunk, "wippy.llm.google:mapper_test", map[string]*typemanifest.Manifest{
		"google_mapper": mapperManifest,
	})
	if len(session.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want named loop finite-container frontier diagnostics", session.Diagnostics)
	}
}

func roundTripFacadeManifest(t *testing.T, name string, m *typemanifest.Manifest) *typemanifest.Manifest {
	t.Helper()
	data, err := typemanifest.Encode(m)
	if err != nil {
		t.Fatalf("Encode %s: %v", name, err)
	}
	decoded, err := typemanifest.Decode(data)
	if err != nil {
		t.Fatalf("Decode %s: %v", name, err)
	}
	return decoded
}
