package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/access"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCheckRunsActiveDiagnostics(t *testing.T) {
	result := Check(`local x: number = "no"`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Position.File != "test.lua" {
		t.Fatalf("diagnostic file = %q, want test.lua", result.Diagnostics[0].Position.File)
	}
}

func TestCheckFilePreservesDiagnosticFilename(t *testing.T) {
	result := CheckFile(`local x: number = "no"`, "main.lua")
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Position.File != "main.lua" {
		t.Fatalf("diagnostic file = %q, want main.lua", result.Diagnostics[0].Position.File)
	}
}

func TestCheckDiagnosticPolicyControlsCode(t *testing.T) {
	disabled := Check(`local x: number = "no"`, WithDiagnosticRule(diagnostics.CodeAssignmentType, diagnostic.Disable()))
	if len(disabled.Diagnostics) != 0 {
		t.Fatalf("disabled diagnostics = %#v, want none", disabled.Diagnostics)
	}

	remapped := Check(`local x: number = "no"`, WithDiagnosticRule(
		diagnostics.CodeAssignmentType,
		diagnostic.OverrideSeverity(diagnostic.SeverityHint),
	))
	if len(remapped.Diagnostics) != 1 {
		t.Fatalf("remapped diagnostics = %#v, want one diagnostic", remapped.Diagnostics)
	}
	if remapped.Diagnostics[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("severity = %s, want hint", remapped.Diagnostics[0].Severity)
	}
}

func TestCheckCanUseJudgmentDirectCallArguments(t *testing.T) {
	result := Check(`local function need_string(value: string): ()
end
need_string(42)`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeDirectCallArgType)
	}
	if !strings.Contains(diag.Message, "argument 1 is 42, not string") {
		t.Fatalf("message = %q, want judgment-rendered argument mismatch", diag.Message)
	}
}

func TestCheckCanUseJudgmentDirectCallContract(t *testing.T) {
	result := Check(`local x: number = 1
x()`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallNotCallable {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallNotCallable)
	}
	if !strings.Contains(result.Diagnostics[0].Message, "x is 1, not callable") {
		t.Fatalf("message = %q, want judgment-rendered callee mismatch", result.Diagnostics[0].Message)
	}
}

func TestOrderedManifestsSkipsNilModuleManifest(t *testing.T) {
	cfg := config{modules: map[string]*ModuleResult{
		"broken": {},
	}}
	if got := cfg.orderedManifests(); len(got) != 0 {
		t.Fatalf("ordered manifests = %#v, want nil module manifest skipped", got)
	}
}

func TestImportedLookupTableLiteralKeyKeepsExportedStaticMemberPrecision(t *testing.T) {
	mapper := CheckFileAndExport(`
type FinishReasonMap = {[string]: string}
local M = {}

local finish_reasons: FinishReasonMap = {}
finish_reasons["end_turn"] = "stop"
finish_reasons["max_tokens"] = "length"
M.finish_reasons = finish_reasons

function M.map_finish_reason(api_reason: string): string
	return M.finish_reasons[api_reason] or "unknown"
end

return M
`, "mapper", "mapper.lua", WithStdlib())
	if len(mapper.Errors) != 0 {
		t.Fatalf("mapper diagnostics = %#v, want none", mapper.Errors)
	}
	export, ok := mapper.Manifest.Export.(*typ.Record)
	if !ok {
		t.Fatalf("mapper export = %T %[1]v, want record", mapper.Manifest.Export)
	}
	field := export.GetField("finish_reasons")
	if field == nil {
		t.Fatalf("mapper export fields = %#v, want finish_reasons", export.Fields)
	}
	if field.Optional {
		t.Fatalf("mapper finish_reasons optional = true, want dominating export assignment present")
	}
	indexed, ok := access.RuntimeIndex(field.Type, typ.LiteralString("end_turn"))
	if !ok || !typ.TypeEquals(indexed, typ.LiteralString("stop")) {
		t.Fatalf("manifest finish_reasons[end_turn] = %v/%v, want literal \"stop\"", indexed, ok)
	}

	result := CheckFile(`
local mapper = require("mapper")
local direct: string = mapper.finish_reasons["end_turn"]
`, "main.lua", WithStdlib(), WithModule("mapper", mapper))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("main diagnostics = %#v, want none", result.Diagnostics)
	}
}

func TestOptionalMapReadIntoUnionAliasReportsNilObligation(t *testing.T) {
	result := Check(`
type Task = {kind: "task", id: string}
type Timer = {kind: "timer", id: string}
type Envelope = Task | Timer
type State = {processed: {[string]: Envelope}}

local state: State = {processed = {}}
state.processed["known"] = {kind = "task", id = "known"}
local missing: Envelope = state.processed["missing"]
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
	}
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{})
	if !strings.Contains(rendered, `state.processed["missing"] can be`) ||
		!strings.Contains(rendered, "nil") ||
		!strings.Contains(rendered, "indexed read that can miss") {
		t.Fatalf("rendered diagnostic missing nil/index evidence:\n%s", rendered)
	}
}

func TestOptionalMapReadFromAnnotatedConstructorResultIntoUnionAliasReportsNilObligation(t *testing.T) {
	result := Check(`
type Task = {kind: "task", id: string}
type Timer = {kind: "timer", id: string}
type Envelope = Task | Timer
type State = {processed: {[string]: Envelope}, counters: {[string]: number}}
type Actor = {state: State}

local function new_actor(): Actor
	return {state = {processed = {}, counters = {}}}
end

local actor = new_actor()
actor.state.processed["known"] = {kind = "task", id = "known"}
actor.state.counters["known"] = 1
local missing_processed: Envelope = actor.state.processed["missing"]
local missing_counter: number = actor.state.counters["missing"]
`)
	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v; processed source: %s",
			len(result.Diagnostics), result.Diagnostics, localAssignmentSourceDebugAtLine(t, result, 15))
	}
	rendered := diagnostic.Render(result.Diagnostics[0], diagnostic.RenderOptions{}) + "\n" +
		diagnostic.Render(result.Diagnostics[1], diagnostic.RenderOptions{})
	if !strings.Contains(rendered, `actor.state.processed["missing"] can be`) ||
		!strings.Contains(rendered, `actor.state.counters["missing"] can be`) {
		t.Fatalf("rendered diagnostics missing optional map read evidence:\n%s", rendered)
	}
}

func TestCheckCanEnableUnusedLocalWarning(t *testing.T) {
	defaultResult := Check(`local unused = 1`)
	if len(defaultResult.Diagnostics) != 0 {
		t.Fatalf("default diagnostics = %#v, want unused-local off by default", defaultResult.Diagnostics)
	}

	warn := Check(`local unused = 1`, WithDiagnosticRule(
		diagnostics.CodeUnusedLocal,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(warn.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unused-local diagnostic", warn.Diagnostics)
	}
	if warn.Diagnostics[0].Code != diagnostics.CodeUnusedLocal || warn.Diagnostics[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("diagnostic = %#v, want unused-local hint", warn.Diagnostics[0])
	}
	requireEvidenceMessage(t, warn.Diagnostics[0], `no read of local "unused" was found in this scope`)
}

func TestCheckCanEnableDeadAssignmentWarning(t *testing.T) {
	defaultResult := Check(`
local value = 1
value = 2
return value
`)
	if len(defaultResult.Diagnostics) != 0 {
		t.Fatalf("default diagnostics = %#v, want dead-assignment off by default", defaultResult.Diagnostics)
	}

	warn := Check(`
local value = 1
value = 2
return value
`, WithDiagnosticRule(
		diagnostics.CodeDeadAssignment,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(warn.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one dead-assignment diagnostic", warn.Diagnostics)
	}
	if warn.Diagnostics[0].Code != diagnostics.CodeDeadAssignment || warn.Diagnostics[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("diagnostic = %#v, want dead-assignment hint", warn.Diagnostics[0])
	}
	requireEvidenceMessage(t, warn.Diagnostics[0], `later assignment replaces "value" before the earlier value is read`)
}

func TestCheckCanEnableRedundantConditionWarning(t *testing.T) {
	src := `
local value = true
if value then
	if value then
		return value
	end
end
`
	defaultResult := Check(src)
	if len(defaultResult.Diagnostics) != 0 {
		t.Fatalf("default diagnostics = %#v, want redundant-condition off by default", defaultResult.Diagnostics)
	}

	warn := Check(src, WithDiagnosticRule(
		diagnostics.CodeRedundantCondition,
		diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	))
	if len(warn.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one redundant-condition diagnostic", warn.Diagnostics)
	}
	if warn.Diagnostics[0].Code != diagnostics.CodeRedundantCondition || warn.Diagnostics[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("diagnostic = %#v, want redundant-condition hint", warn.Diagnostics[0])
	}
	requireEvidenceMessage(t, warn.Diagnostics[0], "value is unchanged between the prior guard and this check")
}

func TestCheckAliasedObjectLiteralMemberReadUsesHeapIdentity(t *testing.T) {
	result := Check(`
local user = { id = "u1" }
local alias = user
local ok_id: string = alias.id
local bad_id: number = alias.id
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one bad_id diagnostic", result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestCheckAliasedNestedObjectLiteralMemberReadUsesNestedHeapIdentity(t *testing.T) {
	result := Check(`
local user = { profile = { id = "u1" } }
local profile = user.profile
local ok_id: string = profile.id
local bad_id: number = profile.id
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one bad_id diagnostic", result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

// A method reached through a recursive type's self-referential field must
// resolve to the field type's method and yield its declared return type. A
// recursive alias resolves to a closed mu-type whose self-references project
// structurally; without that, the member call falls back to any and the
// string assignment fails.
func TestCheckRecursiveSelfFieldMethodResolvesReturn(t *testing.T) {
	result := Check(`
type Node = {
    name: string,
    child: Node?,
    label: (self: Node) -> string,
}
local function make_node(name: string): Node
    return {
        name = name,
        child = nil,
        label = function(self: Node): string
            return self.name
        end,
    }
end
local root = make_node("root")
if root.child then
    local name: string = root.child:label()
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want 0: %#v", len(result.Diagnostics), result.Diagnostics)
	}
}

func TestCheckAndExportReturnsManifestAndDiagnostics(t *testing.T) {
	mod := CheckAndExport(`local x: string = 42`, "mod")
	if mod == nil || mod.Manifest == nil {
		t.Fatal("CheckAndExport did not return module manifest")
	}
	if len(mod.Errors) != 1 {
		t.Fatalf("module errors = %d, want 1: %#v", len(mod.Errors), mod.Errors)
	}
	if mod.Errors[0].Position.File != "mod" {
		t.Fatalf("module diagnostic file = %q, want mod", mod.Errors[0].Position.File)
	}
}

func TestCheckAndExportPublishesRootObjectLiteralExport(t *testing.T) {
	mod := CheckAndExport(`return { value = 1 }`, "mod")
	if mod == nil || mod.Manifest == nil {
		t.Fatal("CheckAndExport did not return module manifest")
	}
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	rec, ok := mod.Manifest.Export.(*typ.Record)
	if !ok {
		t.Fatalf("export = %T %[1]v, want record", mod.Manifest.Export)
	}
	field := rec.GetField("value")
	if field == nil {
		t.Fatalf("export fields = %#v, want value field", rec.Fields)
	}
	if !typ.TypeEquals(field.Type, typ.LiteralInt(1)) {
		t.Fatalf("value field type = %v, want literal 1", field.Type)
	}
}

func TestCheckAndExportPublishesReturnedTableDottedFunctionMember(t *testing.T) {
	mod := CheckAndExport(`
		local provider = {}
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	fn := requireFunctionField(t, requireExportRecord(t, mod), "meta")
	if len(fn.Returns) != 1 {
		t.Fatalf("meta returns = %d, want 1", len(fn.Returns))
	}
	ret, ok := fn.Returns[0].(*typ.Record)
	if !ok {
		t.Fatalf("meta return = %T %[1]v, want record", fn.Returns[0])
	}
	field := ret.GetField("name")
	if field == nil {
		t.Fatalf("meta return fields = %#v, want name", ret.Fields)
	}
	if !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("name field type = %v, want string", field.Type)
	}
}

func TestCheckAndExportPublishesReturnedTableDottedFunctionMemberMultiReturns(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.fetch(id: string): (number?, string?)
			if id == "" then
				return nil, "missing"
			end
			return 1, nil
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	fn := requireFunctionField(t, requireExportRecord(t, mod), "fetch")
	if len(fn.Returns) != 2 {
		t.Fatalf("fetch returns = %d, want 2", len(fn.Returns))
	}
	if !typ.TypeEquals(fn.Returns[0], typeexpr.Optional(typ.Number)) {
		t.Fatalf("fetch return 1 = %v, want number?", fn.Returns[0])
	}
	if !typ.TypeEquals(fn.Returns[1], typeexpr.Optional(typ.String)) {
		t.Fatalf("fetch return 2 = %v, want string?", fn.Returns[1])
	}
	sig, ok := mod.Manifest.FunctionSignatures["client.fetch"]
	if !ok {
		t.Fatalf("missing client.fetch function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if !typ.TypeEquals(sig.Type, fn) {
		t.Fatalf("signature type = %v, want exported fetch type %v", sig.Type, fn)
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("signature type = %v effect = %v, want ErrorReturn(0, 1)", sig.Type, sig.Effect)
	}
}

func TestCheckAndExportUnannotatedFunctionReturnKeepsAssignedOptionalField(t *testing.T) {
	registry := CheckFileAndExport(`
type Entry = {
    id: string,
    data: {[string]: any},
}

local pages = {}

local function qualify_id(entry_id: string, relative_id: string): string
    return entry_id .. ":" .. relative_id
end

function pages.build_page(entry: Entry)
    local raw_data_func = entry.data.data_func
    local data_func: string? = nil
    if type(raw_data_func) == "string" and raw_data_func ~= "" then
        data_func = qualify_id(entry.id, raw_data_func)
    end

    local page = {}
    page.data_func = data_func
    return page
end

return pages
`, "page_registry", "page_registry.lua", WithStdlib())
	if len(registry.Errors) != 0 {
		t.Fatalf("registry diagnostics = %#v, want none", registry.Errors)
	}
	sig, ok := registry.Manifest.FunctionSignatures["page_registry.build_page"]
	if !ok || sig.Type == nil || len(sig.Type.Returns) != 1 {
		t.Fatalf("missing page_registry.build_page return signature: %#v", registry.Manifest.FunctionSignatures)
	}
	wantReturn := typetable.NewRecord().
		Field("data_func", typeexpr.Optional(typ.String)).
		Build()
	if !typ.TypeEquals(sig.Type.Returns[0], wantReturn) {
		t.Fatalf("build_page return = %v, want %v", sig.Type.Returns[0], wantReturn)
	}

	result := CheckFile(`
local page_registry = require("page_registry")

local function get_page_data(page)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end

    local name: string = page.data_func
    return {name}, nil
end

local page = page_registry.build_page({
    id = "demo",
    data = { data_func = "load_data" },
})

return get_page_data(page)
`, "main.lua", WithStdlib(), WithModule("page_registry", registry))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, export = %v, want guarded returned field to stay string?", result.Diagnostics, registry.Manifest.Export)
	}
}

func TestRequireCheckAndExportedAssignedFunctionLiteralUsesErrorReturnSignature(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		client.fetch = function(id: string): (number?, string?)
			if id == "" then
				return nil, "missing"
			end
			return 1, nil
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	if _, ok := mod.Manifest.FunctionSignatures["client.fetch"]; !ok {
		t.Fatalf("missing client.fetch function signature: %#v", mod.Manifest.FunctionSignatures)
	}

	result := Check(`
		local client = require("client")
		local value, err = client.fetch("id")
		if err == nil then
			local n: number = value
		end
	`, WithStdlib(), WithModule("client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported error-return correlation", result.Diagnostics)
	}
}

func TestRequireCheckAndExportedAssignedImportedFunctionAliasPublishesCallableShape(t *testing.T) {
	runtime := manifest.New("runtime")
	storeType := typ.Func().
		Param("item", typ.String).
		Returns(typ.Number).
		Build()
	runtime.SetExport(typ.Unknown)
	runtime.DefineFunctionSignature("runtime.store", signature.Function{Type: storeType})

	mod := CheckAndExport(`
		local runtime = require("runtime")
		local M = {}
		local store = runtime.store
		M.store = store
		return M
	`, "storage", WithStdlib(), WithManifest("runtime", runtime))
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	requireFunctionField(t, requireExportRecord(t, mod), "store")

	result := Check(`
		local storage = require("storage")
		local n: number = storage.store("item")
	`, WithStdlib(), WithModule("storage", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want assigned imported function alias callable", result.Diagnostics)
	}
}

func TestCheckAndExportPublishesNormalReturnAbsentParamRefinement(t *testing.T) {
	mod := CheckAndExport(`
		local test = {}
		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "expected nil", 2)
			end
		end
		return test
	`, "test", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	sig, ok := mod.Manifest.FunctionSignatures["test.is_nil"]
	if !ok {
		t.Fatalf("missing test.is_nil function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if !hasNormalReturnAbsentRefinement(sig.Effect, 0) {
		t.Fatalf("signature effect = %v, want normal return absent refinement for param 0", sig.Effect)
	}
	if hasNormalReturnAbsentRefinement(sig.Effect, 1) {
		t.Fatalf("signature effect = %v, did not expect absent refinement for msg param", sig.Effect)
	}
}

func TestCheckAndExportPublishesNormalReturnPresentParamRefinement(t *testing.T) {
	mod := CheckAndExport(`
		local test = {}
		function test.not_nil(val: any, msg: string?)
			if val == nil then
				error(msg or "expected non-nil", 2)
			end
		end
		return test
	`, "test", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	sig, ok := mod.Manifest.FunctionSignatures["test.not_nil"]
	if !ok {
		t.Fatalf("missing test.not_nil function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if !hasNormalReturnPresentRefinement(sig.Effect, 0) {
		t.Fatalf("signature effect = %v, want normal return present refinement for param 0", sig.Effect)
	}
	if hasNormalReturnPresentRefinement(sig.Effect, 1) {
		t.Fatalf("signature effect = %v, did not expect present refinement for msg param", sig.Effect)
	}
}

func TestCheckAndExportPrefersReturnedTableSourceMembersOverShallowSummary(t *testing.T) {
	mod := CheckAndExport(`
		local provider: { value: number } = { value = 1 }
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	rec := requireExportRecord(t, mod)
	field := rec.GetField("value")
	if field == nil {
		t.Fatalf("export fields = %#v, want value field", rec.Fields)
	}
	if !typ.TypeEquals(field.Type, typ.LiteralInt(1)) {
		t.Fatalf("value field type = %v, want literal 1", field.Type)
	}
	requireFunctionField(t, rec, "meta")
}

func TestRequireCheckAndExportedStaticStringFunctionMemberNamesResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		client["fetch"] = function(): string
			return "ok"
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local client = require("client")
		local n: number = client["fetch"]()
	`, WithStdlib(), WithModule("client", mod))
	requireAssignmentDiagnosticWithEvidence(t, result, "static string imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], `client["fetch"](...) has type string`)
	requireEvidenceMessage(t, result.Diagnostics[0], "n is declared as number")
}

func TestRequireCheckAndExportedStaticIntFunctionMemberNamesResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local mod = {}
		mod[1] = function(): string
			return "ok"
		end
		return mod
	`, "runtime")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local runtime = require("runtime")
		local n: number = runtime[1]()
	`, WithStdlib(), WithModule("runtime", mod))
	requireAssignmentDiagnosticWithEvidence(t, result, "static int imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "runtime[1](...) has type string")
	requireEvidenceMessage(t, result.Diagnostics[0], "n is declared as number")
}

func TestRequireCheckAndExportedMultiReturnMemberNamesResultEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.fetch(id: string): (number?, string?)
			return nil, "missing"
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local client = require("client")
		local value: number?, err: number = client.fetch("id")
	`, WithStdlib(), WithModule("client", mod))
	requireAssignmentDiagnosticWithEvidence(t, result, "imported member multi-return result")
	requireEvidenceMessage(t, result.Diagnostics[0], "client.fetch(...) can be string or nil here")
	requireEvidenceMessage(t, result.Diagnostics[0], "err is declared as number")
	requireEvidenceMessage(t, result.Diagnostics[0], "no guard on this path proves client.fetch(...) is non-nil")
}

func TestRequireCheckAndExportedReturnedTableDottedMemberKeepsMultiReturns(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.fetch(id: string): (number?, string?)
			return nil, "missing"
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local client = require("client")
		local value, err = client.fetch("id")
		local e: number = err
	`, WithStdlib(), WithModule("client", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedReturnedTableDottedMemberUsesSignatureErrorReturnCorrelation(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.fetch(id: string): (number?, string?)
			if id == "" then
				return nil, "missing"
			end
			return 1, nil
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local client = require("client")
		local value, err = client.fetch("id")
		if err == nil then
			local n: number = value
		end
	`, WithStdlib(), WithModule("client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after imported error-return correlation", result.Diagnostics)
	}
}

func TestRequireCheckAcceptsImportedRecordAssignmentsAndReturns(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Snapshot = { id: string }
		local M = {}
		M.Snapshot = Snapshot
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local snapshot: protocol.Snapshot = { id = "u1" }
		local function make_snapshot(): protocol.Snapshot
			return { id = "u2" }
		end
	`, WithStdlib(), WithModule("protocol", protocolMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for imported record assignment and return", checked.Diagnostics)
	}
}

func TestCheckRejectsAnnotatedLocalFunctionExpressionParamMismatch(t *testing.T) {
	checked := Check(`
		type User = { id: string, retries: number }
		type Audit = { user_id: string, event: string }
		type AuditResult = { ok: true, value: Audit } | { ok: false, error: string }
		type UserAuditHandler = (User) -> AuditResult
		local wrong_handler: UserAuditHandler = function(audit: Audit): AuditResult
			return { ok = true, value = audit }
		end
	`)
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one function parameter mismatch diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedIsNilUsesNormalReturnRefinementForSiblingReturn(t *testing.T) {
	testMod := CheckAndExport(`
		local test = {}
		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "expected nil", 2)
			end
		end
		return test
	`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	clientMod := CheckAndExport(`
		local client = {}
		type Response = {
			metadata: {
				response_id: string,
			},
		}
		function client.request(ok: boolean): (Response?, string?)
			if ok then
				return {
					metadata = {
						response_id = "resp-123",
					},
				}, nil
			end
			return nil, "failed"
		end
		return client
	`, "client", WithStdlib())
	if len(clientMod.Errors) != 0 {
		t.Fatalf("client module errors = %#v, want none", clientMod.Errors)
	}

	result := Check(`
		local test = require("test")
		local client = require("client")
		local response, err = client.request(true)
		test.is_nil(err, "no error expected")
		local id: string = response.metadata.response_id
	`, WithStdlib(), WithModule("test", testMod), WithModule("client", clientMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after imported is_nil normal-return refinement", result.Diagnostics)
	}
}

func TestRequireCheckAndExportedNotNilUsesNormalReturnRefinementForValueReturn(t *testing.T) {
	testMod := CheckAndExport(`
		local test = {}
		function test.not_nil(val: any, msg: string?)
			if val == nil then
				error(msg or "expected non-nil", 2)
			end
		end
		return test
	`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	clientMod := CheckAndExport(`
		local client = {}
		type Response = {
			metadata: {
				response_id: string,
			},
		}
		function client.request(ok: boolean): (Response?, string?)
			if ok then
				return {
					metadata = {
						response_id = "resp-123",
					},
				}, nil
			end
			return nil, "failed"
		end
		return client
	`, "client", WithStdlib())
	if len(clientMod.Errors) != 0 {
		t.Fatalf("client module errors = %#v, want none", clientMod.Errors)
	}

	result := Check(`
		local test = require("test")
		local client = require("client")
		local response, err = client.request(true)
		test.is_nil(err, "no error expected")
		test.not_nil(response, "response expected")
		local id: string = response.metadata.response_id
	`, WithStdlib(), WithModule("test", testMod), WithModule("client", clientMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none after imported not_nil normal-return refinement", result.Diagnostics)
	}
}

func TestRequireCheckAndExportedNotNilRefinesHelperReturnUsedAsMethodReceiver(t *testing.T) {
	testMod := CheckAndExport(`
		local test = {}
		function test.not_nil(val: any, msg: string?): any
			if val == nil then
				error(msg or "expected non-nil", 2)
			end
			return val
		end
		return test
	`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	result := Check(`
		local test = require("test")

		type Binding = {
			get_context: (Binding, { host: { kind: string } }) -> string,
		}

		local function make_binding(): Binding?
			return {
				get_context = function(self: Binding, input: { host: { kind: string } }): string
					return input.host.kind
				end,
			}
		end

		local function open_binding()
			local instance = make_binding()
			test.not_nil(instance, "binding expected")
			return instance
		end

		local value: string = open_binding():get_context({ host = { kind = "session" } })
	`, WithStdlib(), WithModule("test", testMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want helper return refined before method receiver use", result.Diagnostics)
	}
}

func TestRequireCheckAndExportedNotNilRefinesFluentErrorReturnHelperReceiver(t *testing.T) {
	testMod := CheckAndExport(`
		local test = {}
		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "expected nil", 2)
			end
		end
		function test.not_nil(val: any, msg: string?): any
			if val == nil then
				error(msg or "expected non-nil", 2)
			end
			return val
		end
		return test
	`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	contract := manifest.New("contract")
	instanceType := typ.NewInterface("BindingInstance", []typ.Method{
		{
			Name: "get_context",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("input", typetable.NewRecord().
					Field("host", typetable.NewRecord().
						Field("kind", typ.String).
						Build()).
					Build()).
				Returns(typ.String).
				Build(),
		},
	})
	defType := typ.NewInterface("BindingDefinition", nil)
	defType.Methods = []typ.Method{
		{
			Name: "with_actor",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("actor", typ.Any).
				Returns(defType).
				Build(),
		},
		{
			Name: "with_scope",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("scope", typ.Any).
				Returns(defType).
				Build(),
		},
		{
			Name: "open",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("id", typ.String).
				Returns(typeexpr.Optional(instanceType), typeexpr.Optional(typ.String)).
				Build(),
		},
	}
	getType := typ.Func().
		Param("id", typ.String).
		Returns(typeexpr.Optional(defType), typeexpr.Optional(typ.String)).
		Build()
	contract.SetExport(typetable.NewRecord().
		Field("get", getType).
		Build())
	contract.DefineFunctionSignature("get", errorReturnSignature(getType))

	result := Check(`
		local test = require("test")
		local contract = require("contract")

		local function open_binding()
			local def, def_err = contract.get("binding")
			test.is_nil(def_err, "contract.get")
			test.not_nil(def, "definition expected")

			local instance, open_err = def
				:with_actor({})
				:with_scope({})
				:open("binding")
			test.is_nil(open_err, "contract.open")
			test.not_nil(instance, "binding expected")
			return instance
		end

		local value: string = open_binding():get_context({ host = { kind = "session" } })
	`, WithStdlib(), WithModule("test", testMod), WithManifest("contract", contract))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want asserted fluent helper return refined before method receiver use", result.Diagnostics)
	}
}

func TestRequireCheckAndExportedNotNilRefinesFluentHelperWithLocalContractIDs(t *testing.T) {
	testMod := CheckAndExport(`
		local test = {}
		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "expected nil", 2)
			end
		end
		function test.not_nil(val: any, msg: string?): any
			if val == nil then
				error(msg or "expected non-nil", 2)
			end
			return val
		end
		return test
	`, "test", WithStdlib())
	if len(testMod.Errors) != 0 {
		t.Fatalf("test module errors = %#v, want none", testMod.Errors)
	}

	contract := manifest.New("contract")
	instanceType := typ.NewInterface("BindingInstance", []typ.Method{
		{
			Name: "get_context",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("input", typetable.NewRecord().
					Field("host", typetable.NewRecord().
						Field("kind", typ.String).
						Build()).
					Build()).
				Returns(typ.String).
				Build(),
		},
	})
	defType := typ.NewInterface("BindingDefinition", nil)
	defType.Methods = []typ.Method{
		{
			Name: "with_actor",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("actor", typ.Any).
				Returns(defType).
				Build(),
		},
		{
			Name: "with_scope",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("scope", typ.Any).
				Returns(defType).
				Build(),
		},
		{
			Name: "open",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("id", typ.String).
				Returns(typeexpr.Optional(instanceType), typeexpr.Optional(typ.String)).
				Build(),
		},
	}
	getType := typ.Func().
		Param("id", typ.String).
		Returns(typeexpr.Optional(defType), typeexpr.Optional(typ.String)).
		Build()
	contract.SetExport(typetable.NewRecord().
		Field("get", getType).
		Build())
	contract.DefineFunctionSignature("get", errorReturnSignature(getType))

	result := Check(`
		local test = require("test")
		local contract = require("contract")
		local CONTRACT_ID = "wippy.agent:run_context"
		local BINDING_ID = "wippy.session.run_context:binding"

		local function open_binding()
			local def, def_err = contract.get(CONTRACT_ID)
			test.is_nil(def_err, "contract.get")
			test.not_nil(def, "definition expected")

			local instance, open_err = def
				:with_actor({})
				:with_scope({})
				:open(BINDING_ID)
			test.is_nil(open_err, "contract.open")
			test.not_nil(instance, "binding expected")
			return instance
		end

		local value: string = open_binding():get_context({ host = { kind = "session" } })
	`, WithStdlib(), WithModule("test", testMod), WithManifest("contract", contract))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local contract ID constants to preserve asserted fluent helper return", result.Diagnostics)
	}
}

func TestCheckAndExportPublishesReturnedTableFunctionMemberParams(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	fn := requireFunctionField(t, requireExportRecord(t, mod), "invoke")
	if len(fn.Params) != 3 {
		t.Fatalf("invoke params = %#v, want 3 params", fn.Params)
	}
	if fn.Params[0].Name != "model_id" || !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		t.Fatalf("first invoke param = %#v, want model_id: string", fn.Params[0])
	}
}

func TestRequireCheckAndExportedReturnedTableDottedMemberChecksArgs(t *testing.T) {
	mod := CheckFileAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
			return { ok = true }, nil
		end
		return client
	`, "bedrock_client", "bedrock_client.lua")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local client = require("bedrock_client")
		client.invoke(42, {}, {})
	`, WithStdlib(), WithModule("bedrock_client", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallArgType)
	}
}

func TestCheckHelperSummaryObligationChecksImportedMemberForwardedArg(t *testing.T) {
	mod := CheckFileAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
			return { ok = true }, nil
		end
		return client
	`, "bedrock_client", "bedrock_client.lua")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := CheckFile(`
		local bedrock_client = require("bedrock_client")
		local function helper(client, model_id)
			return client.invoke(model_id, {}, {})
		end
		local contract_args = nil :: any
		local model_id = contract_args.model
		helper(bedrock_client, model_id)
	`, "main.lua", WithModule("bedrock_client", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallArgType)
	}
	requireEvidenceMessage(t, result.Diagnostics[0], "inside helper, argument 1 (model_id) is passed to client.invoke parameter 1, which requires string")
}

func TestCheckHelperSummaryUsesFieldCarriedImportedProviderMember(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
			return {}
		end
		return client
	`, "bedrock_client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	clean := Check(`
		local bedrock_client = require("bedrock_client")
		local handler = {
			_client = bedrock_client,
		}
		local function helper(client, model_id)
			return client.invoke(model_id, {}, {})
		end
		helper(handler._client, "amazon.titan-embed-text-v2:0")
	`, WithStdlib(), WithModule("bedrock_client", mod))
	if len(clean.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want field-carried provider member to stay callable", clean.Diagnostics)
	}

	mismatch := Check(`
		local bedrock_client = require("bedrock_client")
		local handler = {
			_client = bedrock_client,
		}
		local function helper(client, model_id)
			return client.invoke(model_id, {}, {})
		end
		local contract_args = nil :: any
		helper(handler._client, contract_args.model)
	`, WithStdlib(), WithModule("bedrock_client", mod))
	if len(mismatch.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(mismatch.Diagnostics), mismatch.Diagnostics)
	}
	if mismatch.Diagnostics[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", mismatch.Diagnostics[0].Code, diagnostics.CodeDirectCallArgType)
	}
}

func TestCheckHelperSummaryKeepsUnannotatedFieldCarriedImportedProviderMemberCallable(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.invoke(model_id, payload, options)
			return {}
		end
		return client
	`, "bedrock_client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local bedrock_client = require("bedrock_client")
		local handler = {
			_client = bedrock_client,
		}
		local function helper(client, model_id)
			return client.invoke(model_id, {}, {})
		end
		helper(handler._client, "amazon.titan-embed-text-v2:0")
	`, WithStdlib(), WithModule("bedrock_client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unannotated field-carried provider member to stay callable", result.Diagnostics)
	}
}

func TestCheckUnannotatedFactoryReturnKeepsStringFallbackField(t *testing.T) {
	http := manifest.New("http_client")
	http.SetExport(typetable.NewRecord().
		Field("get", typ.Func().
			Param("url", typ.String).
			Param("options", typ.Any).
			Returns(typ.Any).
			Build()).
		Build())

	result := Check(`
		local http_client = require("http_client")

		local function resolve_config()
			local config = {
				base_url = nil or "https://api.example.test",
				timeout = 600,
			}
			return config
		end

		local function request(endpoint_path: string)
			local config = resolve_config()
			local full_url = config.base_url .. endpoint_path
			return http_client.get(full_url, { timeout = config.timeout })
		end

		request("/v1/messages")
	`, WithStdlib(), WithManifest("http_client", http))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want unannotated factory return to preserve string fallback field", result.Diagnostics)
	}
}

func TestCheckUnannotatedFactoryReturnKeepsNestedOptionalStringFallbackField(t *testing.T) {
	http := manifest.New("http_client")
	http.SetExport(typetable.NewRecord().
		Field("get", typ.Func().
			Param("url", typ.String).
			Param("options", typ.Any).
			Returns(typ.Any).
			Build()).
		Build())

	result := Check(`
		local http_client = require("http_client")

		local function resolve_config()
			local ctx_all = nil :: any
			local function resolve_string(key: string, default_env: string?): string?
				if ctx_all[key] then
					return tostring(ctx_all[key])
				end
				return nil
			end
			local config = {
				base_url = resolve_string("base_url", "BASE_URL") or "https://api.example.test",
				timeout = tonumber(resolve_string("timeout", "TIMEOUT")) or 600,
			}
			return config
		end

		local function request(endpoint_path: string)
			local config = resolve_config()
			local full_url = config.base_url .. endpoint_path
			return http_client.get(full_url, { timeout = config.timeout })
		end

		request("/v1/messages")
	`, WithStdlib(), WithManifest("http_client", http))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want nested optional helper fallback fields to survive factory return", result.Diagnostics)
	}
}

func TestCheckHelperSummaryObligationChecksArithmeticParam(t *testing.T) {
	provider := CheckAndExport(`
		local provider = {}
		function provider.meta(): {name: string}
			return {name = "model"}
		end
		return provider
	`, "provider")
	if len(provider.Errors) != 0 {
		t.Fatalf("provider errors = %#v, want none", provider.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local function scale(tokens)
			return tokens * 4
		end
		local m = provider.meta()
		scale(m)
	`, WithStdlib(), WithModule("provider", provider))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallArgType)
	}
}

func TestCheckAndExportPublishesReturnedTableMethodFunctionMember(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client:invoke(model_id: string): string
			return model_id
		end
		return client
	`, "client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	fn := requireFunctionField(t, requireExportRecord(t, mod), "invoke")
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("invoke returns = %#v, want string", fn.Returns)
	}
}

func TestCheckAndExportPublishesLocalTypeDefinitions(t *testing.T) {
	mod := CheckAndExport(`
		type User = { name: string }
		return { value = 1 }
	`, "mod")
	if mod == nil || mod.Manifest == nil {
		t.Fatal("CheckAndExport did not return module manifest")
	}
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	got, ok := mod.Manifest.Types["User"]
	if !ok {
		t.Fatalf("manifest types = %#v, want User", mod.Manifest.Types)
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("User type = %T %[1]v, want record", got)
	}
	field := rec.GetField("name")
	if field == nil {
		t.Fatalf("User fields = %#v, want name field", rec.Fields)
	}
	if !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("name field type = %v, want string", field.Type)
	}
}

func TestCheckAndExportedTypeAliasResolvesInImporterWithoutValueField(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string }
		return { version = "v1" }
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	if _, ok := protocolMod.Manifest.Types["User"]; !ok {
		t.Fatalf("manifest types = %#v, want User type export", protocolMod.Manifest.Types)
	}
	if rec := requireExportRecord(t, protocolMod); rec.GetField("User") != nil {
		t.Fatalf("export fields = %#v, did not expect type alias as value field", rec.Fields)
	}

	result := Check(`
		local protocol = require("protocol")
		local user: protocol.User = { id = "u1" }
		local wrong: protocol.User = { id = 42 }
	`, WithStdlib(), WithModule("protocol", protocolMod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one type mismatch: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestCheckAndExportPublishesDirectAssignedRHSObjectShape(t *testing.T) {
	mod := CheckAndExport(`
		local Widget = {}
		function Widget.new(): { id: string }
			return { id = "w1" }
		end
		local M = {}
		M.Widget = Widget
		return M
	`, "widget", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local widget = require("widget")
		local made = widget.Widget.new()
		local id: string = made.id
		local wrong: number = made.id
	`, WithStdlib(), WithModule("widget", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one wrong id diagnostic: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedBranchMemberAssignmentIsNotRequired(t *testing.T) {
	for _, tc := range []struct {
		name   string
		assign string
		read   string
	}{
		{
			name:   "dot",
			assign: "M.value = 42",
			read:   "mod.value",
		},
		{
			name:   "static string",
			assign: `M["value"] = 42`,
			read:   `mod["value"]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `
				local M = {}
				if cond then
					return M
				end
				` + tc.assign + `
				return M
			`
			mod := CheckAndExport(src, "mod", WithGlobals("cond"))
			if len(mod.Errors) != 0 {
				t.Fatalf("module errors = %#v, want none", mod.Errors)
			}

			result := Check(`
				local mod = require("mod")
				local n: number = `+tc.read+`
			`, WithStdlib(), WithModule("mod", mod))
			requireAssignmentDiagnosticWithEvidence(t, result, fmt.Sprintf("export = %v, read = %s", mod.Manifest.Export, tc.read))
		})
	}
}

func TestRequireCheckAndExportedBranchFunctionDefinitionIsNotRequired(t *testing.T) {
	mod := CheckAndExport(`
		local M = {}
		if cond then
			return M
		end
		function M.value(): number
			return 42
		end
		return M
	`, "mod", WithGlobals("cond"))
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local mod = require("mod")
		local n: number = mod.value()
	`, WithStdlib(), WithModule("mod", mod))
	requireMemberCallDiagnosticWithEvidence(t, result, diagnostics.CodeMissingMember, fmt.Sprintf("export = %v, call = mod.value()", mod.Manifest.Export))
}

func TestRequireCheckAndExportedConditionalMemberAssignmentIsNotRequired(t *testing.T) {
	for _, tc := range []struct {
		name   string
		assign string
		read   string
	}{
		{
			name:   "dot",
			assign: "M.value = 42",
			read:   "mod.value",
		},
		{
			name:   "static string",
			assign: `M["value"] = 42`,
			read:   `mod["value"]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := CheckAndExport(`
				local M = {}
				if cond then
					`+tc.assign+`
				end
				return M
			`, "mod", WithGlobals("cond"))
			if len(mod.Errors) != 0 {
				t.Fatalf("module errors = %#v, want none", mod.Errors)
			}

			result := Check(`
				local mod = require("mod")
				local n: number = `+tc.read+`
			`, WithStdlib(), WithModule("mod", mod))
			requireAssignmentDiagnosticWithEvidence(t, result, fmt.Sprintf("export = %v, read = %s", mod.Manifest.Export, tc.read))
		})
	}
}

func TestRequireCheckAndExportedStaticStringOptionalMemberHasBoundaryEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "conditional assignment",
			src: `
				local M = {}
				if cond then
					M["value"] = 42
				end
				return M
			`,
		},
		{
			name: "early return before assignment",
			src: `
				local M = {}
				if cond then
					return M
				end
				M["value"] = 42
				return M
			`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := CheckAndExport(tc.src, "mod", WithGlobals("cond"))
			if len(mod.Errors) != 0 {
				t.Fatalf("module errors = %#v, want none", mod.Errors)
			}

			result := Check(`
				local mod = require("mod")
				local n: number = mod["value"]
			`, WithStdlib(), WithModule("mod", mod))
			requireAssignmentDiagnosticWithEvidence(t, result, fmt.Sprintf("export = %v, read = mod[\"value\"]", mod.Manifest.Export))
			requireEvidenceMessage(t, result.Diagnostics[0], "mod[\"value\"] is an indexed read that can miss or read nil")
			requireEvidenceMessage(t, result.Diagnostics[0], "no proof shows the selected slot satisfies the declared type here")
		})
	}
}

func TestRequireCheckAndExportedStaticStringOptionalMemberNilGuardNarrowsAssignment(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "conditional assignment",
			src: `
				local M = {}
				if cond then
					M["value"] = 42
				end
				return M
			`,
		},
		{
			name: "early return before assignment",
			src: `
				local M = {}
				if cond then
					return M
				end
				M["value"] = 42
				return M
			`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := CheckAndExport(tc.src, "mod", WithGlobals("cond"))
			if len(mod.Errors) != 0 {
				t.Fatalf("module errors = %#v, want none", mod.Errors)
			}

			result := Check(`
				local mod = require("mod")
				if mod["value"] ~= nil then
					local n: number = mod["value"]
				end
			`, WithStdlib(), WithModule("mod", mod))
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want none after mod[\"value\"] nil guard", result.Diagnostics)
			}
		})
	}
}

func TestRequireCheckAndExportedStaticStringOptionalMemberGuardVariants(t *testing.T) {
	mod := CheckAndExport(`
		local M = {}
		if cond then
			M["value"] = 42
		end
		return M
	`, "mod", WithGlobals("cond"))
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "local alias nil guard",
			src: `
				local mod = require("mod")
				local value = mod["value"]
				if value ~= nil then
					local n: number = value
				end
			`,
		},
		{
			name: "direct truthy guard",
			src: `
				local mod = require("mod")
				if mod["value"] then
					local n: number = mod["value"]
				end
			`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(tc.src, WithStdlib(), WithModule("mod", mod))
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want none after %s", result.Diagnostics, tc.name)
			}
		})
	}
}

func TestRequireCheckAndExportedConditionalFunctionDefinitionIsNotRequired(t *testing.T) {
	mod := CheckAndExport(`
		local M = {}
		if cond then
			function M.value(): number
				return 42
			end
		end
		return M
	`, "mod", WithGlobals("cond"))
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local mod = require("mod")
		local n: number = mod.value()
	`, WithStdlib(), WithModule("mod", mod))
	requireMemberCallDiagnosticWithEvidence(t, result, diagnostics.CodeNotCallable, fmt.Sprintf("export = %v, call = mod.value()", mod.Manifest.Export))
}

func TestRequireCheckAndExportedStaticIntMemberCallReportsNonCallable(t *testing.T) {
	mod := CheckAndExport(`
		local M = {}
		M[1] = 42
		return M
	`, "mod")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local mod = require("mod")
		mod[1]()
	`, WithStdlib(), WithModule("mod", mod))
	requireMemberCallDiagnosticWithEvidence(t, result, diagnostics.CodeNotCallable, fmt.Sprintf("export = %v, call = mod[1]()", mod.Manifest.Export))
}

func TestRequireCheckAndExportedStaticIntFunctionMemberChecksArgs(t *testing.T) {
	mod := CheckAndExport(`
		local M = {}
		M[1] = function(value: string): number
			return 1
		end
		return M
	`, "mod")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	if _, ok := mod.Manifest.FunctionSignatures["mod[1]"]; !ok {
		t.Fatalf("function signatures = %#v, want mod[1]", mod.Manifest.FunctionSignatures)
	}

	result := Check(`
		local mod = require("mod")
		local n: number = mod[1](42)
	`, WithStdlib(), WithModule("mod", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one argument diagnostic: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s: %#v", diag.Code, diagnostics.CodeDirectCallArgType, diag)
	}
	requireEvidenceMessage(t, diag, "mod[1] parameter 1 expects string")
}

func TestRequireCheckAndExportedMultiSourceRootKeepsReturnedTableSourceMember(t *testing.T) {
	mod := CheckAndExport(`
		local provider: { value: number } = { value = 1 }
		function provider.meta(): { name: string }
			return { name = "model" }
		end
		return provider, "tag"
	`, "provider")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local provider = require("provider")
		local meta = provider.meta()
		local name: string = meta.name
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, export = %v, want none", result.Diagnostics, mod.Manifest.Export)
	}
}

func TestCheckAndExportMultiSourceRootUsesFirstReturnSlot(t *testing.T) {
	mod := CheckAndExport(`return "primary", { ignored = true }`, "mod")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	if !typ.TypeEquals(mod.Manifest.Export, typ.LiteralString("primary")) {
		t.Fatalf("export = %T %[1]v, want first return slot literal", mod.Manifest.Export)
	}
}

func TestCheckAndExportDoesNotPublishIgnoredReturnSlotFunctionSignatures(t *testing.T) {
	mod := CheckAndExport(`
		local primary = { value = 1 }
		local ignored = {}
		function ignored.bad(x: string)
		end
		return primary, ignored
	`, "mod")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}
	if _, ok := mod.Manifest.FunctionSignatures["mod.bad"]; ok {
		t.Fatalf("leaked ignored second-return signature: %#v", mod.Manifest.FunctionSignatures)
	}
}

func TestRequireCheckAndExportedMultiSourceSkippedReturnKeepsAbsentMemberEvidence(t *testing.T) {
	mod := CheckAndExport(`
		local M = {}
		if cond then
			return M, "tag"
		end
		M.value = 42
		return M
	`, "mod", WithGlobals("cond"))
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local mod = require("mod")
		local n: number = mod.value
	`, WithStdlib(), WithModule("mod", mod))
	requireAssignmentDiagnosticWithEvidence(t, result, fmt.Sprintf("export = %v, multi-source early return should keep maybe-absent value", mod.Manifest.Export))
}

func requireAssignmentDiagnosticWithEvidence(t *testing.T, result Result, context string) {
	t.Helper()
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one possibly-absent assignment diagnostic (%s): %#v", len(result.Diagnostics), context, result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s (%s): %#v", diag.Code, diagnostics.CodeAssignmentType, context, diag)
	}
	if len(diag.Explanation.Evidence()) == 0 {
		t.Fatalf("diagnostic evidence = %#v, want assignment evidence chain (%s)", diag.Explanation.Evidence(), context)
	}
}

func requireMemberCallDiagnosticWithEvidence(t *testing.T, result Result, want diagnostic.Code, context string) {
	t.Helper()
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one %s diagnostic (%s): %#v", len(result.Diagnostics), want, context, result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != want {
		t.Fatalf("diagnostic code = %s, want %s (%s): %#v", diag.Code, want, context, diag)
	}
	if len(diag.Explanation.Evidence()) == 0 {
		t.Fatalf("diagnostic evidence = %#v, want member-call evidence (%s)", diag.Explanation.Evidence(), context)
	}
	for _, evidence := range diag.Explanation.Evidence() {
		if strings.Contains(evidence.Message, "has receiver type") ||
			strings.Contains(evidence.Message, "has type") ||
			strings.Contains(evidence.Message, "has literal value") {
			return
		}
	}
	t.Fatalf("diagnostic evidence = %#v, want path/type member-call evidence (%s)", diag.Explanation.Evidence(), context)
}

func requireDirectCallResultDiagnosticWithEvidence(t *testing.T, result Result, context string) {
	t.Helper()
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one direct-call result diagnostic (%s): %#v", len(result.Diagnostics), context, result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeDirectCallResultAssignment {
		t.Fatalf("diagnostic code = %s, want %s (%s): %#v", diag.Code, diagnostics.CodeDirectCallResultAssignment, context, diag)
	}
	if len(diag.Explanation.Evidence()) < 2 {
		t.Fatalf("diagnostic evidence = %#v, want direct-call result evidence chain (%s)", diag.Explanation.Evidence(), context)
	}
}

func requireEvidenceMessage(t *testing.T, diag diagnostic.Diagnostic, want string) {
	t.Helper()
	for _, evidence := range diag.Explanation.Evidence() {
		if strings.Contains(evidence.Message, want) {
			return
		}
	}
	t.Fatalf("diagnostic evidence = %#v, want message containing %q", diag.Explanation.Evidence(), want)
}

func requireLabelMessage(t *testing.T, diag diagnostic.Diagnostic, want string) {
	t.Helper()
	for _, label := range diag.Labels {
		if label.Message == want {
			return
		}
	}
	t.Fatalf("diagnostic labels = %#v, want label %q", diag.Labels, want)
}

func TestCheckAndExportPublishesAssignedMetatableClassTableShape(t *testing.T) {
	mod := CheckAndExport(`
		type Widget = {
			label: (self: Widget) -> string,
		}
		local Widget = {}
		Widget.__index = Widget
		function Widget.new(): Widget
			local self: Widget = {
				label = Widget.label,
			}
			setmetatable(self, Widget)
			return self
		end
		function Widget:label(): string
			return "ok"
		end
		local M = {}
		M.Widget = Widget
		M.new = Widget.new
		return M
	`, "widget", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	export := requireExportRecord(t, mod)
	classField := export.GetField("Widget")
	if classField == nil {
		t.Fatalf("export fields = %#v, want Widget class table field", export.Fields)
	}
	requireFunctionFieldFromType(t, classField.Type, "new")
	requireFunctionFieldFromType(t, classField.Type, "label")
	requireFunctionField(t, export, "new")

	result := Check(`
		local widget = require("widget")
		local instance = widget.new()
		local label: string = instance:label()
		local wrong: number = instance:label()
	`, WithStdlib(), WithModule("widget", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one wrong label diagnostic: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestCheckAndExportUnsupportedModuleStaysUnknownNotAny(t *testing.T) {
	mod := CheckAndExport(`
		break
		return { value = 1 }
	`, "unsupported")
	if len(mod.Errors) == 0 {
		t.Fatal("module errors = none, want structural evidence for unsupported module")
	}
	if mod == nil || mod.Manifest == nil {
		t.Fatal("CheckAndExport did not return module manifest")
	}
	if !typ.IsUnknown(mod.Manifest.Export) {
		t.Fatalf("export = %T %[1]v, want unknown", mod.Manifest.Export)
	}
	if typ.IsAny(mod.Manifest.Export) {
		t.Fatalf("export = %v, did not expect any fallback", mod.Manifest.Export)
	}
}

func TestCheckDoesNotReportUnsupportedCFGAsTypeDiagnostic(t *testing.T) {
	result := Check(`
		local t = {}
		function t:m()
		end
	`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unsupported active CFG coverage", result.Diagnostics)
	}
}

func TestWithManifestThreadsFunctionSignaturesIntoActiveChecking(t *testing.T) {
	m := manifest.New("test")
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().Returns(typ.Number).Build(),
	})

	mismatch := Check(`local x: string = f()`, WithManifest("test", m), WithGlobals("f"))
	if len(mismatch.Diagnostics) != 1 {
		t.Fatalf("mismatch diagnostics = %d, want 1: %#v", len(mismatch.Diagnostics), mismatch.Diagnostics)
	}
	if mismatch.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", mismatch.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
	requireEvidenceMessage(t, mismatch.Diagnostics[0], "f(...) has type number")
	requireEvidenceMessage(t, mismatch.Diagnostics[0], "x is declared as string")

	matching := Check(`local x: number = f()`, WithManifest("test", m), WithGlobals("f"))
	if len(matching.Diagnostics) != 0 {
		t.Fatalf("matching diagnostics = %#v, want none", matching.Diagnostics)
	}
}

func TestWithManifestDoesNotResolveLocalAliasByName(t *testing.T) {
	m := manifest.New("test")
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().Returns(typ.Number).Build(),
	})

	result := Check(`
		local f = function()
			return 1
		end
		local x: string = f()
	`, WithManifest("test", m), WithGlobals("f"))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want local-function assignment mismatch only", result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestWithManifestResolvesExplicitDottedGlobalStaticCalleePathOnly(t *testing.T) {
	m := manifest.New("test")
	m.DefineFunctionSignature("pkg.make", signature.Function{
		Type: typ.Func().Returns(typ.Number).Build(),
	})

	globalMismatch := Check(`local x: string = pkg.make()`, WithManifest("test", m), WithGlobals("pkg"))
	if len(globalMismatch.Diagnostics) != 1 {
		t.Fatalf("global diagnostics = %d, want 1: %#v", len(globalMismatch.Diagnostics), globalMismatch.Diagnostics)
	}
	if globalMismatch.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("global diagnostic code = %s, want %s", globalMismatch.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
	requireEvidenceMessage(t, globalMismatch.Diagnostics[0], "pkg.make(...) has type number")
	requireEvidenceMessage(t, globalMismatch.Diagnostics[0], "x is declared as string")

	localRoot := Check(`
		local pkg = {}
		local x: string = pkg.make()
	`, WithManifest("test", m), WithGlobals("pkg"))
	if len(localRoot.Diagnostics) != 0 {
		t.Fatalf("local-root diagnostics = %#v, want none", localRoot.Diagnostics)
	}
}

func TestWithManifestReportsExplicitDottedGlobalTooFewArgs(t *testing.T) {
	m := manifest.New("test")
	m.DefineFunctionSignature("pkg.make", signature.Function{
		Type: typ.Func().
			Param("name", typ.String).
			Param("count", typ.Number).
			Build(),
	})

	result := Check(`pkg.make("only-one")`, WithManifest("test", m), WithGlobals("pkg"))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallTooFewArgs {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallTooFewArgs)
	}
}

func TestWithManifestReportsExplicitDottedGlobalTooManyArgs(t *testing.T) {
	m := manifest.New("test")
	m.DefineFunctionSignature("pkg.make", signature.Function{
		Type: typ.Func().
			Param("name", typ.String).
			Param("count", typ.Number).
			Build(),
	})

	result := Check(`pkg.make("name", 1, true)`, WithManifest("test", m), WithGlobals("pkg"))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallTooManyArgs {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallTooManyArgs)
	}
	if len(result.Diagnostics[0].Labels) != 1 || result.Diagnostics[0].Labels[0].Message != "extra argument" {
		t.Fatalf("labels = %#v, want extra argument label", result.Diagnostics[0].Labels)
	}
}

func TestEffectOnlyImportedMemberSignatureDoesNotSuppressStructuralCallDiagnostic(t *testing.T) {
	m := manifest.New("pkg")
	m.SetExport(typetable.NewRecord().Field("run", typ.Number).Build())
	m.DefineFunctionSignature("pkg.run", signature.Function{})

	result := Check(`
		local pkg: {run: number}? = require("pkg")
		pkg.run()
	`, WithStdlib(), WithManifest("pkg", m))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeNotCallable {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeNotCallable)
	}
	requireEvidenceMessage(t, result.Diagnostics[0], "pkg.run has type number at call")
}

func TestAnyMemberCallReportsMissingCallableProof(t *testing.T) {
	result := Check(`
local function compile(config: {context_merger: any}): ()
    config.context_merger({}, {}, {})
end
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one non-callable any member diagnostic", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeNotCallable {
		t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeNotCallable)
	}
	if !strings.Contains(diag.Message, "config.context_merger comes from any/unknown") ||
		!strings.Contains(diag.Message, "no proof shows it is callable") {
		t.Fatalf("diagnostic message = %q, want callable proof-boundary message", diag.Message)
	}
}

func TestTypedImportedMemberSignatureAcceptsPresentRequireReceiverDespiteOptionalAnnotation(t *testing.T) {
	m := manifest.New("pkg")
	runType := typ.Func().Build()
	m.SetExport(typetable.NewRecord().Field("run", runType).Build())
	m.DefineFunctionSignature("pkg.run", signature.Function{Type: runType})

	result := Check(`
		local pkg: {run: () -> ()}? = require("pkg")
		pkg.run()
	`, WithStdlib(), WithManifest("pkg", m))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for present require receiver", result.Diagnostics)
	}
}

func TestTypedImportedMemberSignatureDoesNotSuppressOptionalMemberDiagnostic(t *testing.T) {
	m := manifest.New("pkg")
	runType := typ.Func().Build()
	m.SetExport(typetable.NewRecord().OptField("run", runType).Build())
	m.DefineFunctionSignature("pkg.run", signature.Function{Type: runType})

	result := Check(`
		local pkg = require("pkg")
		pkg.run()
	`, WithStdlib(), WithManifest("pkg", m))
	requireMemberCallDiagnosticWithEvidence(t, result, diagnostics.CodeNotCallable, "optional imported member with typed signature")
}

func TestTypedImportedMemberSignatureDoesNotSuppressNonCallableMemberDiagnostic(t *testing.T) {
	m := manifest.New("pkg")
	m.SetExport(typetable.NewRecord().Field("run", typ.Number).Build())
	m.DefineFunctionSignature("pkg.run", signature.Function{Type: typ.Func().Build()})

	result := Check(`
		local pkg = require("pkg")
		pkg.run()
	`, WithStdlib(), WithManifest("pkg", m))
	requireMemberCallDiagnosticWithEvidence(t, result, diagnostics.CodeNotCallable, "non-callable imported member with typed signature")
}

func TestImportedStaticIntMemberCallReportsNonCallableWithEvidence(t *testing.T) {
	m := manifest.New("pkg")
	m.SetExport(typetable.NewRecord().StaticIntIndex(1, typ.Number).Build())

	result := Check(`
		local pkg = require("pkg")
		pkg[1]()
	`, WithStdlib(), WithManifest("pkg", m))
	requireMemberCallDiagnosticWithEvidence(t, result, diagnostics.CodeNotCallable, "non-callable imported static int member")
}

func TestImportedStaticIntMemberCallReportsMissingMemberWithEvidence(t *testing.T) {
	m := manifest.New("pkg")
	m.SetExport(typetable.NewRecord().StaticIntIndex(1, typ.Func().Build()).Build())

	result := Check(`
		local pkg = require("pkg")
		pkg[2]()
	`, WithStdlib(), WithManifest("pkg", m))
	requireMemberCallDiagnosticWithEvidence(t, result, diagnostics.CodeMissingMember, "missing imported static int member")
}

func TestRequireManifestExportTypesImportedValueAndMemberCall(t *testing.T) {
	m := providerManifest("provider")

	ok := Check(`
		local provider = require("provider")
		local n: number = provider.value
		local s: string = provider.meta()
	`, WithStdlib(), WithManifest("provider", m))
	if len(ok.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for typed imported provider", ok.Diagnostics)
	}

	mismatch := Check(`
		local provider = require("provider")
		local n: number = provider.meta()
	`, WithStdlib(), WithManifest("provider", m))
	requireAssignmentDiagnosticWithEvidence(t, mismatch, "typed imported provider member result")
	requireEvidenceMessage(t, mismatch.Diagnostics[0], "provider.meta(...) has type string")
	requireEvidenceMessage(t, mismatch.Diagnostics[0], "n is declared as number")
}

func TestImportedOptionalMethodZeroArgReadUsesExportedOperationalErrorReturn(t *testing.T) {
	mod := CheckAndExport(`
type Stream = {
	read: (self: Stream, n: number?) -> (string?, string?),
}

type Response = {
	status_code: number,
	body: string?,
	stream: Stream?,
}

local http_client = {}

function http_client.get(url: string): (Response?, string?)
	local stream: Stream = {
		read = function(self: Stream, n: number?)
			return "chunk", nil
		end,
	}

	return {
		status_code = 500,
		stream = stream,
	}, nil
end

return http_client
`, "http_client", WithStdlib())
	if len(mod.Errors) != 0 {
		t.Fatalf("module diagnostics = %#v, want none", mod.Errors)
	}
	sig, ok := mod.Manifest.FunctionSignatures["http_client.get"]
	if !ok {
		t.Fatalf("missing http_client.get signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if sig.OperationalEffects == nil {
		t.Fatalf("http_client.get operational effects = nil")
	}
	assertSignatureReturnPresenceRelation(t, sig.OperationalEffects.ReturnPresenceRelations, 0, presence.Present(), 1, presence.Absent())
	assertSignatureReturnPresenceRelation(t, sig.OperationalEffects.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())

	result := Check(`
local http_client = require("http_client")

local response, err = http_client.get("https://example.test")
if err or not response then
	return nil, err
end

if response.status_code >= 300 and response.stream and not response.body then
	local body_data = response.stream:read()
	response.body = body_data
end

return response
`, WithStdlib(), WithModule("http_client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

func TestRequireManifestExportFailsClosedForUnknownOrDynamicPath(t *testing.T) {
	m := providerManifest("provider")

	unknown := Check(`
			local provider = require("missing")
			local n: number = provider.meta()
		`, WithStdlib(), WithManifest("provider", m))
	unknownDiag := requireDiagnosticCode(t, unknown, diagnostics.CodeAssignmentType)
	requireEvidenceMessage(t, unknownDiag, "provider.meta(...) has type any")
	requireEvidenceMessage(t, unknownDiag, "n is declared as number")
	requireEvidenceMessage(t, unknownDiag, "no proof on this path shows provider.meta(...) satisfies the declared type")

	dynamic := Check(`
		local provider = require(module_name)
		local n: number = provider.meta()
	`, WithStdlib(), WithManifest("provider", m), WithGlobals("module_name"))
	diag := requireDiagnosticCode(t, dynamic, diagnostics.CodeDirectCallArgType)
	requireEvidenceMessage(t, diag, "argument 1 (module_name) has type any")
	requireEvidenceMessage(t, diag, "require parameter 1 expects string")
	requireEvidenceMessage(t, diag, "no proof on this path shows module_name satisfies the parameter type")
}

func TestRequireManifestExportMatchesExactPathOnly(t *testing.T) {
	provider := providerManifest("provider")
	fooProvider := providerManifest("foo.provider")

	for _, tc := range []struct {
		name string
		call string
		m    *manifest.Manifest
	}{
		{name: "provider does not match foo.provider", call: "foo.provider", m: provider},
		{name: "foo.provider does not match provider", call: "provider", m: fooProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(fmt.Sprintf(`
					local provider = require(%q)
					local n: number = provider.meta()
				`, tc.call), WithStdlib(), WithManifest(tc.m.Path, tc.m))
			diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
			requireEvidenceMessage(t, diag, "provider.meta(...) has type any")
			requireEvidenceMessage(t, diag, "n is declared as number")
			requireEvidenceMessage(t, diag, "no proof on this path shows provider.meta(...) satisfies the declared type")
		})
	}
}

func TestManifestPathsAndSignatureRootsAreNotGlobalImports(t *testing.T) {
	m := providerManifest("provider")
	m.DefineFunctionSignature("provider.meta", signature.Function{
		Type: typ.Func().Returns(typ.String).Build(),
	})

	result := Check(`local n: number = provider.meta()`, WithStdlib(), WithManifest("provider", m))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want unresolved provider only: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeUnresolvedValueReference {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeUnresolvedValueReference)
	}
}

func TestCheckAndExportUsesCapturedImportedRootSignature(t *testing.T) {
	jsonMod := CheckAndExport(`
		local json = {}
		function json.decode(src: string): any
			return {}
		end
		return json
	`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	clientMod := CheckAndExport(`
		local json = require("json")
		local client = {}
		function client.decode()
			return json.decode(42)
		end
		return client
	`, "client", WithStdlib(), WithModule("json", jsonMod))
	if len(clientMod.Errors) != 1 {
		t.Fatalf("client module errors = %d, want 1: %#v", len(clientMod.Errors), clientMod.Errors)
	}
	if clientMod.Errors[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", clientMod.Errors[0].Code, diagnostics.CodeDirectCallArgType)
	}
}

func TestCheckAndExportUsesCapturedStaticMemberImportAliasSignature(t *testing.T) {
	httpMod := CheckAndExport(`
		local http_client = {}
		function http_client.get(url: string): any
			return {}
		end
		return http_client
	`, "http_client")
	if len(httpMod.Errors) != 0 {
		t.Fatalf("http_client module errors = %#v, want none", httpMod.Errors)
	}

	clientMod := CheckAndExport(`
		local http_client = require("http_client")
		local client = {
			_http_client = http_client,
		}
		function client.request()
			return client._http_client.get(42)
		end
		return client
	`, "client", WithStdlib(), WithModule("http_client", httpMod))
	if len(clientMod.Errors) != 1 {
		t.Fatalf("client module errors = %d, want 1: %#v", len(clientMod.Errors), clientMod.Errors)
	}
	if clientMod.Errors[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", clientMod.Errors[0].Code, diagnostics.CodeDirectCallArgType)
	}
}

func TestCheckAndExportReexportsImportedOwnershipEffectsAcrossModules(t *testing.T) {
	runtime := manifest.New("runtime")
	runtime.DefineFunctionSignature("runtime.send_one", signature.Function{
		Type: typ.Func().Param("payload", typ.Any).Build(),
		Effect: effect.Empty.With(ownership.SendParam{
			Param: effect.ParamRef{Index: 0},
		}),
	})
	runtime.DefineFunctionSignature("runtime.freeze_one", signature.Function{
		Type: typ.Func().Param("payload", typ.Any).Build(),
		Effect: effect.Empty.With(ownership.Freeze{
			Param: effect.ParamRef{Index: 0},
		}),
	})
	runtime.DefineFunctionSignature("runtime.mutate_one", signature.Function{
		Type: typ.Func().Param("container", typ.Any).Build(),
		Effect: effect.Empty.With(mutation.TableMutator{
			Target: effect.ParamRef{Index: 0},
			Value:  effect.ParamRef{Index: -1},
		}),
	})
	runtime.DefineFunctionSignature("runtime.store_into", signature.Function{
		Type: typ.Func().
			Param("value", typ.Any).
			Param("container", typ.Any).
			Build(),
		Effect: effect.Empty.With(ownership.Store{
			Param: effect.ParamRef{Index: 0},
			Into:  effect.ParamRef{Index: 1},
		}),
	})

	providerMod := CheckAndExport(`
		local provider = {}
		function provider.forward(payload: any)
			runtime.send_one(payload)
		end
		function provider.seal(payload: any)
			runtime.freeze_one(payload)
		end
		function provider.mutate(container: any)
			runtime.mutate_one(container)
		end
		function provider.store(value: any, container: any)
			runtime.store_into(value, container)
		end
		return provider
	`, "provider", WithManifest("runtime", runtime), WithGlobals("runtime"))
	if len(providerMod.Errors) != 0 {
		t.Fatalf("provider module errors = %#v, want none", providerMod.Errors)
	}
	assertExportedSendParam(t, providerMod.Manifest, "provider.forward", 0)
	assertExportedFreeze(t, providerMod.Manifest, "provider.seal", 0)
	assertExportedTableMutator(t, providerMod.Manifest, "provider.mutate", 0)
	assertExportedStoreExact(t, providerMod.Manifest, "provider.store", 0, 1)
	assertNoExportedTableMutator(t, providerMod.Manifest, "provider.store")

	consumerMod := CheckAndExport(`
		local provider = require("provider")
		local consumer = {}
		function consumer.forward(payload: any)
			provider.forward(payload)
		end
		function consumer.seal(payload: any)
			provider.seal(payload)
		end
		function consumer.mutate(container: any)
			provider.mutate(container)
		end
		function consumer.store(value: any, container: any)
			provider.store(value, container)
		end
		return consumer
	`, "consumer", WithStdlib(), WithModule("provider", providerMod))
	if len(consumerMod.Errors) != 0 {
		t.Fatalf("consumer module errors = %#v, want none", consumerMod.Errors)
	}
	assertExportedSendParam(t, consumerMod.Manifest, "consumer.forward", 0)
	assertExportedFreeze(t, consumerMod.Manifest, "consumer.seal", 0)
	assertExportedTableMutator(t, consumerMod.Manifest, "consumer.mutate", 0)
	assertExportedStoreExact(t, consumerMod.Manifest, "consumer.store", 0, 1)
	assertNoExportedTableMutator(t, consumerMod.Manifest, "consumer.store")
}

func TestCheckAndExportPublishesAssignedImportedFunctionEffect(t *testing.T) {
	storeType := typ.Func().
		Param("value", typ.Any).
		Param("container", typ.Any).
		Build()
	runtime := manifest.New("runtime")
	runtime.SetExport(typetable.NewRecord().Field("store", storeType).Build())
	runtime.DefineFunctionSignature("runtime.store", signature.Function{
		Type: storeType,
		Effect: effect.Empty.With(ownership.Store{
			Param: effect.ParamRef{Index: 0},
			Into:  effect.ParamRef{Index: 1},
		}),
	})

	providerMod := CheckAndExport(`
		local runtime = require("runtime")
		local provider = {}
		provider.store = runtime.store
		return provider
	`, "provider", WithStdlib(), WithManifest("runtime", runtime))
	if len(providerMod.Errors) != 0 {
		t.Fatalf("provider module errors = %#v, want none", providerMod.Errors)
	}

	requireFunctionField(t, requireExportRecord(t, providerMod), "store")
	assertExportedStoreExact(t, providerMod.Manifest, "provider.store", 0, 1)
}

func TestCheckAndExportPublishesLocalAliasOfImportedFunctionEffect(t *testing.T) {
	storeType := typ.Func().
		Param("value", typ.Any).
		Param("container", typ.Any).
		Build()
	runtime := manifest.New("runtime")
	runtime.SetExport(typetable.NewRecord().Field("store", storeType).Build())
	runtime.DefineFunctionSignature("runtime.store", signature.Function{
		Type: storeType,
		Effect: effect.Empty.With(ownership.Store{
			Param: effect.ParamRef{Index: 0},
			Into:  effect.ParamRef{Index: 1},
		}),
	})

	providerMod := CheckAndExport(`
		local runtime = require("runtime")
		local store = runtime.store
		local provider = {}
		provider.store = store
		return provider
	`, "provider", WithStdlib(), WithManifest("runtime", runtime))
	if len(providerMod.Errors) != 0 {
		t.Fatalf("provider module errors = %#v, want none", providerMod.Errors)
	}

	requireFunctionField(t, requireExportRecord(t, providerMod), "store")
	assertExportedStoreExact(t, providerMod.Manifest, "provider.store", 0, 1)
}

func TestCheckStateLanesDisableSingleAnalysisAxis(t *testing.T) {
	src := `
type Job = { id: string }
local job: Job = { id = "route-1" }
process.send("worker-1", "route.ready", job)
`
	opts := []Option{WithStdlib(), WithManifest("process", ProcessManifest()), WithGlobals("process")}
	defaultResult := Check(src, opts...)
	if len(defaultResult.Diagnostics) != 0 {
		t.Fatalf("default diagnostics = %#v, want none", defaultResult.Diagnostics)
	}
	if defaultResult.checked == nil || defaultResult.checked.RootResult() == nil {
		t.Fatal("missing default checked root result")
	}
	defaultExit, ok := defaultResult.checked.RootResult().ExitState()
	if !ok {
		t.Fatal("missing default exit state")
	}
	if shared, stack := placementCounts(defaultExit); shared == 0 && stack == 0 {
		t.Fatalf("default placement counts shared=%d stack=%d, want placement lane populated", shared, stack)
	}

	withoutPlacement := state.DefaultLaneSet().Without(state.LanePlacement).IDs()
	disabledOpts := append(append([]Option{}, opts...), WithStateLanes(withoutPlacement...))
	disabledResult := Check(src, disabledOpts...)
	if len(disabledResult.Diagnostics) != 0 {
		t.Fatalf("disabled placement diagnostics = %#v, want none", disabledResult.Diagnostics)
	}
	if disabledResult.checked == nil || disabledResult.checked.RootResult() == nil {
		t.Fatal("missing disabled checked root result")
	}
	disabledExit, ok := disabledResult.checked.RootResult().ExitState()
	if !ok {
		t.Fatal("missing disabled exit state")
	}
	if shared, stack := placementCounts(disabledExit); shared != 0 || stack != 0 {
		t.Fatalf("disabled placement counts shared=%d stack=%d, want placement lane ignored by constructor slice", shared, stack)
	}
}

func TestOptionsCopyCallerOwnedStorageAtConstruction(t *testing.T) {
	globals := []string{"alpha"}
	globalsOpt := WithGlobals(globals...)
	globals[0] = "beta"

	cfg := applyOptions([]Option{globalsOpt})
	if len(cfg.globals) != 1 || cfg.globals[0] != "alpha" {
		t.Fatalf("WithGlobals captured mutated caller slice: got %#v, want [alpha]", cfg.globals)
	}
	cfg.globals[0] = "gamma"
	cfg = applyOptions([]Option{globalsOpt})
	if len(cfg.globals) != 1 || cfg.globals[0] != "alpha" {
		t.Fatalf("WithGlobals reused applied config storage: got %#v, want [alpha]", cfg.globals)
	}

	policy := diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		diagnostics.CodeAssignmentType: diagnostic.Disable(),
	}}
	policyOpt := WithDiagnosticPolicy(policy)
	policy.Rules[diagnostics.CodeAssignmentType] = diagnostic.Enable()

	cfg = applyOptions([]Option{policyOpt})
	if !cfg.diagnosticPolicy.Rules[diagnostics.CodeAssignmentType].Disabled {
		t.Fatalf("WithDiagnosticPolicy captured mutated caller map: got %#v, want disabled assignment diagnostic", cfg.diagnosticPolicy.Rules)
	}
	cfg.diagnosticPolicy.Rules[diagnostics.CodeAssignmentType] = diagnostic.Enable()
	cfg = applyOptions([]Option{policyOpt})
	if !cfg.diagnosticPolicy.Rules[diagnostics.CodeAssignmentType].Disabled {
		t.Fatalf("WithDiagnosticPolicy reused applied config map: got %#v, want disabled assignment diagnostic", cfg.diagnosticPolicy.Rules)
	}

	lanes := []state.LaneID{state.LaneValues}
	opt := WithStateLanes(lanes...)
	lanes[0] = state.LaneFrozenTables

	cfg = applyOptions([]Option{opt})
	if len(cfg.stateLanes) != 1 || cfg.stateLanes[0] != state.LaneValues {
		t.Fatalf("WithStateLanes captured mutated caller slice: got %#v, want [%s]", cfg.stateLanes, state.LaneValues)
	}

	cfg.stateLanes[0] = state.LaneFrozenTables
	cfg = applyOptions([]Option{opt})
	if len(cfg.stateLanes) != 1 || cfg.stateLanes[0] != state.LaneValues {
		t.Fatalf("WithStateLanes reused applied config storage: got %#v, want [%s]", cfg.stateLanes, state.LaneValues)
	}

	empty := applyOptions([]Option{WithStateLanes()})
	if empty.stateLanes == nil || len(empty.stateLanes) != 0 {
		t.Fatalf("WithStateLanes() = %#v, want non-nil empty slice to disable every axis", empty.stateLanes)
	}
}

func TestCheckStateLanesDisableDiagnosticAnalysisAxis(t *testing.T) {
	src := `
local tx = {}
begin(tx)
`
	defaultResult := Check(src, lifecycleManifestOptions("begin", "finish")...)
	if len(defaultResult.Diagnostics) != 1 {
		t.Fatalf("default diagnostics = %#v, want one unreleased-resource warning", defaultResult.Diagnostics)
	}
	if defaultResult.Diagnostics[0].Code != diagnostics.CodeResourceUnreleased {
		t.Fatalf("default diagnostic code = %s, want %s", defaultResult.Diagnostics[0].Code, diagnostics.CodeResourceUnreleased)
	}

	withoutTypestates := state.DefaultLaneSet().Without(state.LaneTypestates).IDs()
	disabledOpts := append(lifecycleManifestOptions("begin", "finish"), WithStateLanes(withoutTypestates...))
	disabledResult := Check(src, disabledOpts...)
	if len(disabledResult.Diagnostics) != 0 {
		t.Fatalf("disabled typestate diagnostics = %#v, want typestate axis ignored by constructor slice", disabledResult.Diagnostics)
	}
}

func TestCheckStateLanesConstructorAcceptsEveryExportedAxis(t *testing.T) {
	src := `local value = 1
value = value + 1
`
	lanes := state.DefaultLanes()
	if len(lanes) == 0 {
		t.Fatal("DefaultLanes() is empty")
	}
	for _, lane := range lanes {
		lane := lane
		t.Run("only/"+string(lane), func(t *testing.T) {
			result := Check(src, WithStateLanes(lane))
			if len(result.Diagnostics) != 0 {
				t.Fatalf("WithStateLanes(%s) diagnostics = %#v, want none", lane, result.Diagnostics)
			}
		})
		t.Run("without/"+string(lane), func(t *testing.T) {
			selected := state.DefaultLaneSet().Without(lane).IDs()
			result := Check(src, WithStateLanes(selected...))
			if len(result.Diagnostics) != 0 {
				t.Fatalf("WithStateLanes(default without %s) diagnostics = %#v, want none", lane, result.Diagnostics)
			}
		})
	}
	t.Run("empty", func(t *testing.T) {
		result := Check(src, WithStateLanes())
		if len(result.Diagnostics) != 0 {
			t.Fatalf("WithStateLanes(empty) diagnostics = %#v, want none", result.Diagnostics)
		}
	})
}

func TestCheckProcessSendPromotesDeepCallbackBuiltMapEntryPlacement(t *testing.T) {
	result := Check(`
type Meta = {
    route: string,
    shard: string,
}
type Child = {
    id: string,
    meta: Meta,
}
type Item = {
    id: string,
    tags: {[string]: string},
    child: Child,
}
type Batch = {
    items: {[string]: Item},
    count: number,
}

local function build(ids: {string}, fill: (Item, string, number) -> ()): Batch
    local batch: Batch = {items = {}, count = 0}
    for _, id in ipairs(ids) do
        batch.count = batch.count + 1
        local item: Item = {
            id = id,
            tags = {},
            child = {
                id = id,
                meta = {route = "", shard = ""},
            },
        }
        item.tags["phase"] = "constructing"
        fill(item, id, batch.count)
        item.tags["phase"] = "ready"
        batch.items[id] = item
    end
    return batch
end

local batch = build({"route-1", "route-2"}, function(item: Item, id: string, index: number)
    item.child.meta.route = id
    if index == 1 then
        item.child.meta.shard = "primary"
    else
        item.child.meta.shard = "backup"
    end
    item.tags["callback"] = "filled"
end)

if batch.items["route-1"] then
    process.send("worker-1", "route.ready", batch.items["route-1"])
end
`, WithStdlib(), WithManifest("process", ProcessManifest()), WithGlobals("process"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	if result.checked == nil || result.checked.RootResult() == nil {
		t.Fatal("missing checked root result")
	}
	root := result.checked.RootResult()
	exit, ok := root.ExitState()
	if !ok {
		t.Fatal("missing exit state")
	}

	shared, stack := placementCounts(exit)
	debug := callOutcomeDebug(root)
	if shared == 0 {
		t.Fatalf("shared placements = 0, want sent payload placement: %s\ncalls: %s\nreturns: %s", placementSummary(root.Registry(), root.KeySpace(), exit), debug, functionReturnDebug(root))
	}
	if stack == 0 {
		t.Fatalf("stack placements = 0, want non-sent construction scaffolding to remain local: %s\ncalls: %s", placementSummary(root.Registry(), root.KeySpace(), exit), debug)
	}
	if depth := maxSharedPlacementDepth(root.Registry(), exit); depth < 3 {
		t.Fatalf("max shared placement depth = %d, want at least item -> child -> meta: %s\ncalls: %s", depth, placementSummary(root.Registry(), root.KeySpace(), exit), debug)
	}
}

func TestCheckProcessSendPromotesCrossModuleAllocationTemplateMapEntryPlacement(t *testing.T) {
	protocolMod := CheckAndExport(`
type Meta = {
    route: string,
    shard: string,
}
type Child = {
    id: string,
    meta: Meta,
}
type Item = {
    id: string,
    tags: {[string]: string},
    child: Child,
}
type Batch = {
    items: {[string]: Item},
    count: number,
}

local M = {}
M.Meta = Meta
M.Child = Child
M.Item = Item
M.Batch = Batch
return M
`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocolMod.Errors)
	}
	builderMod := CheckAndExport(`
local protocol = require("protocol")

local M = {}

function M.build(ids: {string}, fill: (protocol.Item, string, number) -> ()): protocol.Batch
    local batch: protocol.Batch = {items = {}, count = 0}
    for _, id in ipairs(ids) do
        batch.count = batch.count + 1
        local item: protocol.Item = {
            id = id,
            tags = {},
            child = {
                id = id,
                meta = {route = "", shard = ""},
            },
        }
        item.tags["phase"] = "constructing"
        fill(item, id, batch.count)
        item.tags["phase"] = "ready"
        batch.items[id] = item
    end
    return batch
end

return M
`, "builder", WithStdlib(), WithModule("protocol", protocolMod))
	if len(builderMod.Errors) != 0 {
		t.Fatalf("builder diagnostics = %#v, want none", builderMod.Errors)
	}

	result := Check(`
local builder = require("builder")

local batch = builder.build({"route-1", "route-2"}, function(item, id: string, index: number)
    item.child.meta.route = id
    if index == 1 then
        item.child.meta.shard = "primary"
    else
        item.child.meta.shard = "backup"
    end
    item.tags["callback"] = "filled"
end)

if batch.items["route-1"] then
    process.send("worker-1", "route.ready", batch.items["route-1"])
end
`, WithStdlib(), WithModule("builder", builderMod), WithManifest("process", ProcessManifest()), WithGlobals("process"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	if result.checked == nil || result.checked.RootResult() == nil {
		t.Fatal("missing checked root result")
	}
	root := result.checked.RootResult()
	exit, ok := root.ExitState()
	if !ok {
		t.Fatal("missing exit state")
	}
	if depth := maxSharedPlacementDepth(root.Registry(), exit); depth < 3 {
		t.Fatalf("max shared placement depth = %d, want at least item -> child -> meta: %s\npoints: %s\ncalls: %s\nbatch source: %s", depth, placementSummary(root.Registry(), root.KeySpace(), exit), pointPlacementDebug(root), callOutcomeDebug(root), localAssignmentSourceDebugAtLine(t, result, 4))
	}
}

func TestCheckProcessSendPromotesCrossModuleReturnedRootPlacement(t *testing.T) {
	builderMod := CheckAndExport(`
local M = {}
function M.build(): { items: {[string]: { id: string }} }
    return {items = {["route-1"] = {id = "route-1"}}}
end
return M
`, "builder", WithStdlib())
	if len(builderMod.Errors) != 0 {
		t.Fatalf("builder diagnostics = %#v, want none", builderMod.Errors)
	}

	result := Check(`
local builder = require("builder")
local batch = builder.build()
process.send("worker-1", "route.ready", batch)
`, WithStdlib(), WithModule("builder", builderMod), WithManifest("process", ProcessManifest()), WithGlobals("process"))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	if result.checked == nil || result.checked.RootResult() == nil {
		t.Fatal("missing checked root result")
	}
	root := result.checked.RootResult()
	exit, ok := root.ExitState()
	if !ok {
		t.Fatal("missing exit state")
	}
	if shared, _ := placementCounts(exit); shared == 0 {
		t.Fatalf("shared placements = 0, want returned batch placement: %s\ncalls: %s", placementSummary(root.Registry(), root.KeySpace(), exit), callOutcomeDebug(root))
	}
}

func TestCheckChannelSendPromotesPayloadPlacement(t *testing.T) {
	result := Check(`
type Job = { id: string, meta: { attempt: number } }

local function dispatch(out: Channel<Job>, id: string)
    local job: Job = { id = id, meta = { attempt = 1 } }
    local scratch = { local_only = true }
    scratch.local_only = false
    out:send(job)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	if result.checked == nil || result.checked.RootResult() == nil {
		t.Fatal("missing checked root result")
	}
	root := result.checked.RootResult()
	if len(root.FunctionResults()) != 1 {
		t.Fatalf("function results = %d, want dispatch body", len(root.FunctionResults()))
	}
	fn := root.FunctionResults()[0]
	exit, ok := fn.ExitState()
	if !ok {
		t.Fatal("missing dispatch exit state")
	}
	shared, stack := placementCounts(exit)
	if shared == 0 {
		t.Fatalf("shared placements = 0, want channel-sent job placement: %s\ncalls: %s", placementSummary(fn.Registry(), fn.KeySpace(), exit), callOutcomeDebug(fn))
	}
	if stack == 0 {
		t.Fatalf("stack placements = 0, want channel receiver/local scaffolding not all promoted: %s\ncalls: %s", placementSummary(fn.Registry(), fn.KeySpace(), exit), callOutcomeDebug(fn))
	}
	if depth := maxSharedPlacementDepth(fn.Registry(), exit); depth < 2 {
		t.Fatalf("max shared placement depth = %d, want job -> meta: %s\ncalls: %s", depth, placementSummary(fn.Registry(), fn.KeySpace(), exit), callOutcomeDebug(fn))
	}
	foundSendEscape := false
	for _, point := range fn.Graph().RPO() {
		outcome, ok := fn.CallOutcomeAt(point)
		if !ok {
			continue
		}
		for _, event := range outcome.NormalReturnFacts.EscapeEvents {
			if event.Kind == callboundary.EscapeEventSend && event.Recursive {
				foundSendEscape = true
			}
		}
	}
	if !foundSendEscape {
		t.Fatalf("call outcomes did not contain recursive send escape: %s", callOutcomeDebug(fn))
	}
}

func TestCheckChannelSendRejectsWrongPayloadType(t *testing.T) {
	src := `
type Job = { id: string, meta: { attempt: number } }

local function dispatch(out: Channel<Job>)
    out:send({ id = 1, meta = { attempt = 1 } })
end
`
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		DiagnosticCount: 1,
		Code:            diagnostics.CodeDirectCallArgType,
		Severity:        diagnostic.SeverityError,
		Line:            5,
		Column:          21,
		Span:            diagnostic.Span{StartLine: 5, StartCol: 21, EndLine: 5, EndCol: 21},
		MessageContains: []string{"argument 1.id", "not string"},
		EvidenceMin:     2,
		EvidenceOrdered: []string{
			"argument 1.id has literal value 1",
			"out.send parameter 1.id expects string",
		},
		LabelMin:      1,
		LabelContains: []string{"argument value"},
		HelpContains:  []string{"parameter type"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.call.direct.argument_type]: argument 1.id is 1, not string",
			"--> test.lua:5:21",
			"out:send({ id = 1, meta = { attempt = 1 } })",
			"argument value",
			"because:",
			"1. proven: argument 1.id has literal value 1",
			"2. claimed: out.send parameter 1.id expects string",
			"help:",
		},
		RenderNotContains: []string{
			"^~",
			"want string",
		},
	})
}

func callOutcomeDebug(root *body.Result) string {
	if root == nil || root.Graph() == nil {
		return "<no graph>"
	}
	out := ""
	for _, point := range root.Graph().RPO() {
		site, ok := root.CallSite(point)
		if !ok {
			continue
		}
		signatureText := "no-signature"
		if sig, ok := root.CallSignature(site); ok {
			signatureText = fmt.Sprintf("effect=%v returns=%d", sig.Effect, len(sig.Type.Returns))
		}
		outcomeText := "no-outcome"
		if outcome, ok := root.CallOutcomeAt(point); ok {
			escapeText := ""
			for i, event := range outcome.NormalReturnFacts.EscapeEvents {
				if escapeText != "" {
					escapeText += ","
				}
				escapeText += fmt.Sprintf("%d:%s:%v", i, event.Target.String(), event.Recursive)
			}
			if escapeText == "" {
				escapeText = "none"
			}
			writeText := ""
			for i, write := range outcome.NormalReturnFacts.PersistentPathWrites {
				if writeText != "" {
					writeText += ","
				}
				valueText := ""
				if id, ok := valueIdentity(root.Registry(), write.Value); ok {
					valueText += ":" + id.String()
				}
				if t, ok := typevalue.TypeOf(root.Registry(), write.Value); ok {
					valueText += ":" + t.String()
				}
				writeText += fmt.Sprintf("%d:%s%s", i, write.Path.String(), valueText)
			}
			if writeText == "" {
				writeText = "none"
			}
			resultIDs := ""
			for i, result := range outcome.Results {
				resultText := fmt.Sprintf("%d", i)
				if id, ok := valueIdentity(root.Registry(), result.Value); ok {
					resultText += fmt.Sprintf(":%s", id)
				}
				if t, ok := typevalue.TypeOf(root.Registry(), result.Value); ok {
					resultText += fmt.Sprintf(":%s", t)
				}
				if resultIDs != "" {
					resultIDs += ","
				}
				resultIDs += resultText
			}
			if resultIDs == "" {
				resultIDs = "none"
			}
			outcomeText = fmt.Sprintf("escapes=[%s] writes=[%s] heap=%d results=[%s]", escapeText, writeText, len(outcome.HeapTableObjects), resultIDs)
		}
		stateText := "no-boundary-state"
		if st, ok := root.StateAtBoundary(point); ok {
			placements := st.PlacementsSnapshot()
			escapes := st.EscapeEventsSnapshot()
			stateText = fmt.Sprintf("statePlacements=%d stateEscapes=%d", len(placements.Placements), len(escapes.Facts))
		}
		calleeValue := "calleeValue=none"
		if value, ok := root.PathValueAtBoundary(point, site.CalleePathRef()); ok {
			calleeValue = "calleeValue=yes"
			if id, ok := valueIdentity(root.Registry(), value); ok {
				calleeValue += ":" + id.String()
			}
			if t, ok := typevalue.TypeOf(root.Registry(), value); ok {
				calleeValue += ":" + t.String()
			}
		}
		paths := ""
		sources := ""
		site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
			if sources != "" {
				sources += ","
			}
			sources += fmt.Sprintf("%d:kind=%d expr=%v ref=%d", i, source.Kind, source.HasExpr, source.ExprRef)
			return true
		})
		if sources == "" {
			sources = "no-arg-sources"
		}
		if fact, ok := root.Call(point); ok {
			for _, arg := range fact.Args {
				argPath, ok := root.ExpressionPath(arg)
				if !ok || argPath.IsEmpty() {
					continue
				}
				if paths != "" {
					paths += ","
				}
				identityText := "no-id"
				if value, ok := root.PathValueAtBoundary(point, argPath); ok {
					if id, ok := valueIdentity(root.Registry(), value); ok {
						identityText = id.String()
					}
				}
				paths += fmt.Sprintf("%s:%s", argPath.String(), identityText)
				for parent := argPath.Parent(); !parent.IsEmpty(); parent = parent.Parent() {
					parentIdentity := "no-id"
					if value, ok := root.PathValueAtBoundary(point, parent); ok {
						if id, ok := valueIdentity(root.Registry(), value); ok {
							parentIdentity = id.String()
						}
					}
					paths += fmt.Sprintf("<%s:%s>", parent.String(), parentIdentity)
					if len(parent.Segments) == 0 {
						break
					}
				}
			}
		}
		if paths == "" {
			paths = "no-arg-paths"
		}
		callee := site.CalleePath().String()
		if callee == "" {
			callee = "no-callee-path"
		}
		callee += fmt.Sprintf("#sym%d", site.CalleePathRef().Symbol)
		receiver := "no-receiver"
		if receiverPath, ok := site.ReceiverPath(); ok {
			receiver = receiverPath.String()
			if value, ok := root.PathValueAtBoundary(point, receiverPath); ok {
				if t, ok := typevalue.TypeOf(root.Registry(), value); ok {
					receiver += fmt.Sprintf(":%s", t.String())
				} else {
					receiver += ":no-type"
				}
			} else {
				receiver += ":no-value"
			}
		}
		method := site.MethodName()
		if method == "" {
			method = "no-method"
		}
		if out != "" {
			out += "; "
		}
		out += fmt.Sprintf("p%d %s %s %s %s callee=%s receiver=%s method=%s sources=[%s] args=[%s]", point, signatureText, outcomeText, stateText, calleeValue, callee, receiver, method, sources, paths)
	}
	if out == "" {
		return "<no calls>"
	}
	return out
}

func pointPlacementDebug(root *body.Result) string {
	if root == nil || root.Graph() == nil {
		return "<no graph>"
	}
	out := ""
	for _, point := range root.Graph().RPO() {
		solvedCount := -1
		if st, ok := root.StateAt(point); ok {
			solvedCount = len(st.PlacementsSnapshot().Placements)
		}
		boundaryCount := -1
		if st, ok := root.StateAtBoundary(point); ok {
			boundaryCount = len(st.PlacementsSnapshot().Placements)
		}
		branchText := ""
		if fact, ok := root.BranchCondition(point); ok {
			if value, ok := root.ExpressionValueBeforeBoundary(point, fact.Condition); ok {
				tp := "no-type"
				if gotType, typeOK := typevalue.TypeOf(root.Registry(), value); typeOK {
					tp = gotType.String()
				}
				branchText = fmt.Sprintf(" branch=%s", tp)
			} else {
				branchText = " branch=no-value"
			}
		}
		if out != "" {
			out += "; "
		}
		out += fmt.Sprintf("p%d solved=%d boundary=%d%s", point, solvedCount, boundaryCount, branchText)
	}
	return out
}

func functionReturnDebug(root *body.Result) string {
	if root == nil {
		return "<no root>"
	}
	out := ""
	for _, fn := range append([]*body.Result{root}, root.FunctionResults()...) {
		if fn == nil || fn.Graph() == nil {
			continue
		}
		exit, ok := fn.ExitState()
		if !ok {
			continue
		}
		ret := exit.ReadReturnSlot(fn.Registry(), 0)
		retID := "no-id"
		if id, ok := valueIdentity(fn.Registry(), ret); ok {
			retID = id.String()
		}
		if out != "" {
			out += "; "
		}
		out += fmt.Sprintf("g%d ret0=%s", fn.Graph().ID(), retID)
	}
	if out == "" {
		return "<no functions>"
	}
	return out
}

func placementCounts(st state.State) (shared, stack int) {
	heap := st.HeapTableObjectsSnapshot()
	for id := range heap.Objects {
		switch st.ReadPlacement(id) {
		case placement.SharedHeap:
			shared++
		case placement.Stack:
			stack++
		}
	}
	return shared, stack
}

func maxSharedPlacementDepth(reg *axis.Registry, st state.State) int {
	heap := st.HeapTableObjectsSnapshot()
	var walk func(identity.ID, map[identity.ID]struct{}) int
	walk = func(id identity.ID, seen map[identity.ID]struct{}) int {
		if id == (identity.ID{}) || st.ReadPlacement(id) != placement.SharedHeap {
			return 0
		}
		if _, ok := seen[id]; ok {
			return 0
		}
		object, ok := heap.Objects[id]
		if !ok {
			return 1
		}
		nextSeen := make(map[identity.ID]struct{}, len(seen)+1)
		for seenID := range seen {
			nextSeen[seenID] = struct{}{}
		}
		nextSeen[id] = struct{}{}
		depth := 1
		for _, value := range object.StaticMembers() {
			if child, ok := valueIdentity(reg, value); ok {
				if childDepth := 1 + walk(child, nextSeen); childDepth > depth {
					depth = childDepth
				}
			}
		}
		for _, fact := range object.DynamicIndexFacts() {
			if child, ok := valueIdentity(reg, fact.Value); ok {
				if childDepth := 1 + walk(child, nextSeen); childDepth > depth {
					depth = childDepth
				}
			}
		}
		return depth
	}
	depth := 0
	for id := range heap.Objects {
		if candidate := walk(id, nil); candidate > depth {
			depth = candidate
		}
	}
	return depth
}

func valueIdentity(reg *axis.Registry, value product.Value) (identity.ID, bool) {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || id == (identity.ID{}) {
		return identity.ID{}, false
	}
	return id, true
}

func placementSummary(reg *axis.Registry, ks *keyspace.KeySpace, st state.State) string {
	heap := st.HeapTableObjectsSnapshot()
	out := ""
	for id, object := range heap.Objects {
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("%s=%s", id, st.ReadPlacement(id))
		members := ""
		for member, value := range object.StaticMembers() {
			if members != "" {
				members += ","
			}
			members += fmt.Sprintf("%s:", ks.Format(member))
			if child, ok := valueIdentity(reg, value); ok {
				members += child.String()
			} else {
				members += "no-id"
			}
		}
		for key, fact := range object.DynamicIndexFacts() {
			if members != "" {
				members += ","
			}
			members += fmt.Sprintf("dyn(%s/%s):", ks.Format(key.Table), key.Site)
			if child, ok := valueIdentity(reg, fact.Value); ok {
				members += child.String()
			} else {
				members += "no-id"
			}
		}
		if members != "" {
			out += "[" + members + "]"
		}
	}
	if dynamic := dynamicSummary(reg, ks, st); dynamic != "" {
		out += " dynamic={" + dynamic + "}"
	}
	if out == "" {
		return "<empty>"
	}
	return out
}

func dynamicSummary(reg *axis.Registry, ks *keyspace.KeySpace, st state.State) string {
	snapshot := st.DynamicIndexFactsSnapshot()
	out := ""
	for key, fact := range snapshot.Facts {
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("%s/%s:value-id=", ks.Format(key.Table), key.Site)
		if id, ok := valueIdentity(reg, fact.Value); ok {
			out += id.String()
		} else {
			out += "none"
		}
	}
	return out
}

func providerManifest(path string) *manifest.Manifest {
	m := manifest.New(path)
	m.SetExport(typetable.NewRecord().
		Field("value", typ.Number).
		Field("meta", typ.Func().Returns(typ.String).Build()).
		Build())
	return m
}

func requireExportRecord(t *testing.T, mod *ModuleResult) *typ.Record {
	t.Helper()
	if mod == nil || mod.Manifest == nil {
		t.Fatal("CheckAndExport did not return module manifest")
	}
	rec, ok := mod.Manifest.Export.(*typ.Record)
	if !ok {
		t.Fatalf("export = %T %[1]v, want record", mod.Manifest.Export)
	}
	return rec
}

func requireFunctionField(t *testing.T, rec *typ.Record, name string) *typ.Function {
	t.Helper()
	field := rec.GetField(name)
	if field == nil {
		t.Fatalf("export fields = %#v, want %s field", rec.Fields, name)
	}
	fn, ok := field.Type.(*typ.Function)
	if !ok {
		t.Fatalf("%s field type = %T %[1]v, want function", name, field.Type)
	}
	return fn
}

func requireFunctionFieldFromType(t *testing.T, got typ.Type, name string) *typ.Function {
	t.Helper()
	if rec, ok := got.(*typ.Record); ok {
		return requireFunctionField(t, rec, name)
	}
	if intersection, ok := got.(*typ.Intersection); ok {
		for _, member := range intersection.Members {
			if rec, ok := member.(*typ.Record); ok {
				if field := rec.GetField(name); field != nil {
					return requireFunctionField(t, rec, name)
				}
			}
		}
	}
	t.Fatalf("type %T %[1]v does not expose function field %q", got, name)
	return nil
}

func hasErrorReturn(row effect.Row, valueIndex, errorIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		err, ok := effect.NormalizeLabel(label).(returns.ErrorReturn)
		return ok && err.ValueIndex == valueIndex && err.ErrorIndex == errorIndex
	})
}

func hasNormalReturnAbsentRefinement(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		refinement, ok := effect.NormalizeLabel(label).(postcondition.NormalReturnRefinement)
		if !ok || refinement.Target.Index != paramIndex {
			return false
		}
		return postcondition.Absent{}.Equals(refinement.Refinement)
	})
}

func hasNormalReturnPresentRefinement(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		refinement, ok := effect.NormalizeLabel(label).(postcondition.NormalReturnRefinement)
		if !ok || refinement.Target.Index != paramIndex {
			return false
		}
		return postcondition.Present{}.Equals(refinement.Refinement)
	})
}

func assertSignatureReturnPresenceRelation(
	t *testing.T,
	relations []signature.ReturnPresenceRelation,
	triggerIndex int,
	triggerPresence presence.Value,
	targetIndex int,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerIndex == triggerIndex &&
			presence.Equal(relation.TriggerPresence, triggerPresence) &&
			relation.TargetIndex == targetIndex &&
			presence.Equal(relation.TargetPresence, targetPresence) {
			return
		}
	}
	t.Fatalf("return presence relations = %#v, missing %d/%s -> %d/%s", relations, triggerIndex, triggerPresence, targetIndex, targetPresence)
}

func assertExportedSendParam(t *testing.T, m *manifest.Manifest, name string, paramIndex int) {
	t.Helper()
	sig, ok := m.FunctionSignatures[name]
	if !ok {
		t.Fatalf("missing %s function signature: %#v", name, m.FunctionSignatures)
	}
	if !sig.Effect.Has(func(label effect.Label) bool {
		send, ok := effect.NormalizeLabel(label).(ownership.SendParam)
		return ok && send.Param.Index == paramIndex
	}) {
		t.Fatalf("%s effect = %v, want SendParam(%d)", name, sig.Effect, paramIndex)
	}
}

func assertExportedFreeze(t *testing.T, m *manifest.Manifest, name string, paramIndex int) {
	t.Helper()
	sig, ok := m.FunctionSignatures[name]
	if !ok {
		t.Fatalf("missing %s function signature: %#v", name, m.FunctionSignatures)
	}
	if !sig.Effect.Has(func(label effect.Label) bool {
		freeze, ok := effect.NormalizeLabel(label).(ownership.Freeze)
		return ok && freeze.Param.Index == paramIndex
	}) {
		t.Fatalf("%s effect = %v, want Freeze(%d)", name, sig.Effect, paramIndex)
	}
}

func assertExportedTableMutator(t *testing.T, m *manifest.Manifest, name string, paramIndex int) {
	t.Helper()
	sig, ok := m.FunctionSignatures[name]
	if !ok {
		t.Fatalf("missing %s function signature: %#v", name, m.FunctionSignatures)
	}
	if !sig.Effect.Has(func(label effect.Label) bool {
		mutator, ok := effect.NormalizeLabel(label).(mutation.TableMutator)
		return ok && mutator.Target.Index == paramIndex && mutator.Value.Index == -1
	}) {
		t.Fatalf("%s effect = %v, want TableMutator(%d)", name, sig.Effect, paramIndex)
	}
}

func assertNoExportedTableMutator(t *testing.T, m *manifest.Manifest, name string) {
	t.Helper()
	sig, ok := m.FunctionSignatures[name]
	if !ok {
		t.Fatalf("missing %s function signature: %#v", name, m.FunctionSignatures)
	}
	if sig.Effect.Has(func(label effect.Label) bool {
		_, ok := effect.NormalizeLabel(label).(mutation.TableMutator)
		return ok
	}) {
		t.Fatalf("%s effect = %v, did not expect degraded TableMutator", name, sig.Effect)
	}
}

func assertExportedStoreExact(t *testing.T, m *manifest.Manifest, name string, sourceIndex, intoIndex int) {
	t.Helper()
	sig, ok := m.FunctionSignatures[name]
	if !ok {
		t.Fatalf("missing %s function signature: %#v", name, m.FunctionSignatures)
	}
	if !sig.Effect.Has(func(label effect.Label) bool {
		store, ok := effect.NormalizeLabel(label).(ownership.Store)
		return ok && store.Param.Index == sourceIndex && store.Into.Index == intoIndex
	}) {
		t.Fatalf("%s effect = %v, want Store(%d, %d)", name, sig.Effect, sourceIndex, intoIndex)
	}
	if sig.Effect.Has(func(label effect.Label) bool {
		store, ok := effect.NormalizeLabel(label).(ownership.Store)
		return ok && store.Param.Index == sourceIndex && store.Into.Index == -1
	}) {
		t.Fatalf("%s effect = %v, did not expect degraded Store(%d, unknown)", name, sig.Effect, sourceIndex)
	}
}
