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
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
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

func TestOrderedManifestsSkipsNilModuleManifest(t *testing.T) {
	cfg := config{modules: map[string]*ModuleResult{
		"broken": {},
	}}
	if got := cfg.orderedManifests(); len(got) != 0 {
		t.Fatalf("ordered manifests = %#v, want nil module manifest skipped", got)
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

func TestRequireCheckAndExportedReturnedTableDottedMemberKeepsReturnType(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local n: number = provider.meta()
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallResultAssignment {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallResultAssignment)
	}
}

func TestRequireCheckAndExportedReturnedTableDottedMemberNamesResultEvidence(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local n: number = provider.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "direct imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "provider.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckInjectedContainerMemberKeepsImportedResultEvidence(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local container = { client = provider }
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "container-injected imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "provider.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckInjectedConstructorReturnNamesMemberResultEvidence(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local function new_container(client)
			return { client = client }
		end
		local container = new_container(provider)
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "constructor-returned injected imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckInjectedContainerMemberReassignmentDropsStaleImportedResultEvidence(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local replacement = {}
		function replacement.meta(): number
			return 1
		end
		local container = { client = provider }
		container.client = replacement
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want no stale provider.meta evidence after member reassignment: %#v", len(result.Diagnostics), result.Diagnostics)
	}
}

func TestRequireCheckInjectedContainerMemberReassignmentUsesReplacementResultEvidence(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local replacement = {
			meta = function(): string
				return "replacement"
			end,
		}
		local container = { client = provider }
		container.client = replacement
		local n: number = container.client.meta()
	`, WithStdlib(), WithModule("provider", mod))
	requireDirectCallResultDiagnosticWithEvidence(t, result, "reassigned injected member replacement result")
	requireEvidenceMessage(t, result.Diagnostics[0], "container.client.meta returns string")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
}

func TestRequireCheckNestedFactoryDIDropsStaleBranchButKeepsSiblingEvidence(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local replacement = {}
		function replacement.meta(): number
			return 1
		end
		local function new_layer(client)
			return {
				registry = {
					primary = client,
					backup = client,
				},
			}
		end
		local function expose(layer)
			return {
				api = layer.registry,
			}
		end
		local root = expose(new_layer(provider))
		root.api.primary = replacement
		local ok: number = root.api.primary.meta()
		local bad: number = root.api.backup.meta()
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 1 {
		debug := "<no checked result>"
		if result.checked != nil && result.checked.RootResult() != nil {
			debug = callOutcomeDebug(result.checked.RootResult())
		}
		t.Fatalf("diagnostics = %d, want one nested factory DI diagnostic: %#v\ncalls: %s", len(result.Diagnostics), result.Diagnostics, debug)
	}
	requireDirectCallResultDiagnosticWithEvidence(t, result, "nested factory DI keeps sibling imported member evidence")
	requireEvidenceMessage(t, result.Diagnostics[0], "root.api.backup.meta returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target bad requires number")
}

func TestRequireCheckInjectedHelperReturnKeepsImportedMemberResultType(t *testing.T) {
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

	result := Check(`
		local provider = require("provider")
		local function read_meta(client)
			return client.meta()
		end
		local n: number = read_meta(provider)
	`, WithStdlib(), WithModule("provider", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want one helper-return assignment diagnostic: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s: %#v", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType, result.Diagnostics[0])
	}
}

func TestRequireCheckInjectedHelperReturnKeepsErrorReturnCorrelation(t *testing.T) {
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
		local function load(injected)
			return injected.fetch("id")
		end
		local value, err = load(client)
		if err == nil then
			local n: number = value
		end
	`, WithStdlib(), WithModule("client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want none for helper-preserved value/error correlation: %#v", len(result.Diagnostics), result.Diagnostics)
	}
}

func TestRequireCheckInjectedHelperNonFinalReturnDoesNotExpandImportedMultiReturn(t *testing.T) {
	mod := CheckAndExport(`
		local client = {}
		function client.fetch(id: string): (number?, boolean?)
			if id == "" then
				return nil, true
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
		local function load(injected)
			return injected.fetch("id"), "marker"
		end
		local value, marker = load(client)
		local marker_string: string = marker
	`, WithStdlib(), WithModule("client", mod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want none for adjusted non-final imported multi-return: %#v", len(result.Diagnostics), result.Diagnostics)
	}
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
	requireDirectCallResultDiagnosticWithEvidence(t, result, "static string imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "client.fetch returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
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
	requireDirectCallResultDiagnosticWithEvidence(t, result, "static int imported member result")
	requireEvidenceMessage(t, result.Diagnostics[0], "runtime[1] returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target n requires number")
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
	requireDirectCallResultDiagnosticWithEvidence(t, result, "imported member multi-return result")
	if !strings.Contains(result.Diagnostics[0].Message, "call result 2") {
		t.Fatalf("diagnostic message = %q, want call result 2", result.Diagnostics[0].Message)
	}
	requireEvidenceMessage(t, result.Diagnostics[0], "client.fetch returns")
	requireEvidenceMessage(t, result.Diagnostics[0], "assignment target err requires number")
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

func TestCheckAndExportPublishesErrorReturnFromImportedGenericResultField(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local result = {}
		result.Result = Result
		return result
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	repoMod := CheckAndExport(`
		local result = require("result")
		type User = { id: string, email: string }
		local repo = {}
		function repo.find_by_id(id: string): Result<User>
			if id == "" then
				return { ok = false, error = "missing" }
			end
			return { ok = true, value = { id = id, email = "a@test" } }
		end
		return repo
	`, "repo", WithStdlib(), WithModule("result", resultMod))
	if len(repoMod.Errors) != 0 {
		t.Fatalf("repo module errors = %#v, want none", repoMod.Errors)
	}

	serviceMod := CheckAndExport(`
		local repo = require("repo")
		local service = {}
		function service.get_email(id: string): (string?, string?)
			local r = repo.find_by_id(id)
			if r.ok then
				return r.value.email, nil
			end
			return nil, r.error
		end
		return service
	`, "service", WithStdlib(), WithModule("result", resultMod), WithModule("repo", repoMod))
	if len(serviceMod.Errors) != 0 {
		t.Fatalf("service module errors = %#v, want none", serviceMod.Errors)
	}

	sig, ok := serviceMod.Manifest.FunctionSignatures["service.get_email"]
	if !ok {
		t.Fatalf("missing service.get_email function signature: %#v", serviceMod.Manifest.FunctionSignatures)
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("signature type = %v effect = %v, want ErrorReturn(0, 1)", sig.Type, sig.Effect)
	}
}

func TestCheckAndExportPublishesErrorReturnFromLocalGenericResultField(t *testing.T) {
	mod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		type User = { id: string, email: string }
		local service = {}
		function service.get_email(r: Result<User>): (string?, string?)
			if r.ok then
				return r.value.email, nil
			end
			return nil, r.error
		end
		return service
	`, "service")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	sig, ok := mod.Manifest.FunctionSignatures["service.get_email"]
	if !ok {
		t.Fatalf("missing service.get_email function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("signature type = %v effect = %v, want ErrorReturn(0, 1)", sig.Type, sig.Effect)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesReturn(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}
	sig, ok := resultMod.Manifest.FunctionSignatures["result.map"]
	if !ok {
		t.Fatalf("missing result.map function signature: %#v", resultMod.Manifest.FunctionSignatures)
	}
	if sig.Type == nil || len(sig.Type.TypeParams) != 2 {
		t.Fatalf("result.map signature = %v, want two type params", sig.Type)
	}

	checked := Check(`
		local result = require("result")
		type StringResult = { ok: true, value: string } | { ok: false, error: string }
		local decoded: StringResult = result.ok("name")
		local mapped = result.map(decoded, function(value: string)
			return #value
		end)
		if mapped.ok then
			local n: number = mapped.value
		end
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported generic return instantiated", checked.Diagnostics)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesImportedCallbackReturn(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string, retries: number }
		local M = {}
		M.User = User
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local result = require("result")
		type UserResult = { ok: true, value: protocol.User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local mapped = result.map(decoded, function(user: protocol.User)
			return user.id .. ":" .. tostring(user.retries)
		end)
		if mapped.ok then
			local text: string = mapped.value
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported callback return instantiated", checked.Diagnostics)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureSeedsUnannotatedCallbackParam(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local result = require("result")
		type User = { id: string, retries: number }
		type UserResult = { ok: true, value: User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local mapped = result.map(decoded, function(user)
			return user.id
		end)
		if mapped.ok then
			local id: string = mapped.value
			local wrong_id: number = mapped.value
		end
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_id diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesNestedCallbackResult(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string, retries: number }
		type Audit = { user_id: string, event: string }
		local M = {}
		M.User = User
		M.Audit = Audit
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.and_then<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
			if result.ok then
				return fn(result.value)
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local result = require("result")
		type UserResult = { ok: true, value: protocol.User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local audit = result.and_then(decoded, function(user: protocol.User)
			return result.ok({ user_id = user.id, event = "created" })
		end)
		if audit.ok then
			local event: string = audit.value.event
			local wrong_event: number = audit.value.event
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_event diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureRejectsAnnotatedCallResult(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local result = require("result")
		type StringResult = { ok: true, value: string } | { ok: false, error: string }
		type NumberResult = { ok: true, value: number } | { ok: false, error: string }
		local decoded: StringResult = result.ok("u1")
		local wrong_result: NumberResult = result.map(decoded, function(value: string)
			return value .. ":mapped"
		end)
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one annotated generic call-result diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
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

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesObjectLiteralArgument(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local result = require("result")
		local wrapped = result.ok({ user_id = "u1", event = "created" })
		if wrapped.ok then
			local event: string = wrapped.value.event
			local wrong_event: number = wrapped.value.event
		end
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_event diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesNestedObjectLiteralChannel(t *testing.T) {
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
			schema: {
				witness: {
					decode: (any) -> T,
				},
			},
		}
		local M = {}
		function M.listen_nested<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		function M.receive_map<T, U>(channel: Channel<T>, fn: (T) -> U): U?
			local value, ok = channel:receive()
			if ok then
				return fn(value)
			end
			return nil
		end
		return M
	`, "process")
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local process = require("process")
		type Node = { id: string }
		type Source = { nodes: Channel<Node> }
		local function node_type(): { decode: (any) -> Node }
			return { decode = function(raw: any): Node return { id = tostring(raw) } end }
		end
		local function handle(source: Source)
			local node_ch = process.listen_nested("nodes", {
				channel = source.nodes,
				schema = { witness = node_type() },
			})
			local mapped = process.receive_map(node_ch, function(node)
				local node_id: string = node.id
				local bad_node_id: number = node.id
				return node_id
			end)
			if mapped then
				local id: string = mapped
				local wrong_id: number = mapped
			end
		end
	`, WithStdlib(), WithModule("process", processMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want bad_node_id and wrong_id diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureFeedsChannelReceive(t *testing.T) {
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
		}
		local M = {}
		function M.listen<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		return M
	`, "process")
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local process = require("process")
		type Node = { id: string }
		type Source = { nodes: Channel<Node> }
		local function handle(source: Source)
			local node_ch = process.listen("nodes", {
				channel = source.nodes,
			})
			local node, ok = node_ch:receive()
			if ok then
				local id: string = node.id
				local wrong_id: number = node.id
			end
		end
	`, WithStdlib(), WithModule("process", processMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_id diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedReceiveMapKeepsCallbackContextAfterPriorReceive(t *testing.T) {
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
			schema: {
				witness: {
					decode: (any) -> T,
				},
			},
		}
		local M = {}
		function M.listen_nested<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		function M.receive_map<T, U>(channel: Channel<T>, fn: (T) -> U): U?
			local value, ok = channel:receive()
			if ok then
				return fn(value)
			end
			return nil
		end
		return M
	`, "process")
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local process = require("process")
		type Node = { id: string, children: {Node} }
		type Source = { nodes: Channel<Node> }
		local function node_type(): { decode: (any) -> Node }
			return { decode = function(raw: any): Node return { id = tostring(raw), children = {} } end }
		end
		local function handle(source: Source)
			local node_ch = process.listen_nested("nodes", {
				channel = source.nodes,
				schema = { witness = node_type() },
			})
			local node, node_ok = node_ch:receive()
			if node_ok then
				local node_id: string = node.id
			end
			local mapped = process.receive_map(node_ch, function(decoded)
				local decoded_id: string = decoded.id
				local bad_decoded_id: number = decoded.id
				return decoded_id
			end)
			if mapped then
				local accepted: string = mapped
				local bad_mapped: number = mapped
			end
			local summary = process.receive_map(node_ch, function(decoded)
				return {
					id = decoded.id,
					label = decoded.id .. ":node",
				}
			end)
			if summary then
				local id: string = summary.id
				local label: string = summary.label
				local bad_id: number = summary.id
				local bad_label: number = summary.label
			end
		end
	`, WithStdlib(), WithModule("process", processMod))
	if len(checked.Diagnostics) != 4 {
		t.Fatalf("diagnostics = %#v, want callback/member mismatch diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedReceiveMapSeedsCallbackFromImportedSourceChannel(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type RawRecord = {
			id: string,
			amount: number,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		type Source = {
			records: Channel<RawRecord>,
			nodes: Channel<Node>,
		}
		local M = {}
		function M.raw_record_array_type(): Type<{RawRecord}>
			return {
				decode = function(raw: any): {RawRecord}
					return {{ id = tostring(raw), amount = 1 }}
				end,
			}
		end
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode_map<T, U>(data: string, witness: Type<T>, fn: (T) -> U): U
			return fn(witness.decode(data))
		end
		function M.decode_many_map<T, U>(data: string, witness: Type<{T}>, fn: (T) -> U): {U}
			local out: {U} = {}
			for _, item in ipairs(witness.decode(data)) do
				table.insert(out, fn(item))
			end
			return out
		end
		return M
	`, "json",
		WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithGlobals("channel"),
		WithModule("protocol", protocolMod))
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}
	processMod := CheckAndExport(`
		type ListenOptions<T> = {
			channel: Channel<T>,
			schema: {
				witness: {
					decode: (any) -> T,
				},
			},
		}
		local M = {}
		function M.listen_nested<T>(topic: string, options: ListenOptions<T>): Channel<T>
			return options.channel
		end
		function M.receive_map<T, U>(channel: Channel<T>, fn: (T) -> U): U?
			local value, ok = channel:receive()
			if ok then
				return fn(value)
			end
			return nil
		end
		return M
	`, "process",
		WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithGlobals("channel"),
		WithModule("protocol", protocolMod),
		WithModule("json", jsonMod))
	if len(processMod.Errors) != 0 {
		t.Fatalf("process module errors = %#v, want none", processMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local process = require("process")
		local function handle(source: protocol.Source)
			local root_label = json.decode_map("{}", protocol.node_type(), function(decoded)
				local decoded_id: string = decoded.id
				local bad_decoded_id: number = decoded.id
				return decoded_id
			end)
			local accepted_label: string = root_label
			local bad_label: number = root_label

			local row_labels = json.decode_many_map("[]", protocol.raw_record_array_type(), function(row)
				local row_amount: number = row.amount
				local bad_row_amount: string = row.amount
				return row.id .. tostring(row_amount)
			end)
			local accepted_labels: {string} = row_labels
			local bad_labels: {number} = row_labels

			local node_ch = process.listen_nested("nodes", {
				channel = source.nodes,
				schema = { witness = protocol.node_type() },
			})
			local node, node_ok = node_ch:receive()
			if node_ok then
				local node_id: string = node.id
				for _, child in ipairs(node.children) do
					local child_id: string = child.id
				end
			end
			local mapped = process.receive_map(node_ch, function(decoded)
				local decoded_id: string = decoded.id
				local bad_decoded_id: number = decoded.id
				return decoded_id
			end)
			if mapped then
				local accepted: string = mapped
				local bad_mapped: number = mapped
			end
		end
	`, WithStdlib(),
		WithManifest("channel", ChannelManifest()),
		WithGlobals("channel"),
		WithModule("protocol", protocolMod),
		WithModule("json", jsonMod),
		WithModule("process", processMod))
	if len(checked.Diagnostics) != 6 {
		t.Fatalf("diagnostics = %#v, want json and receive_map mismatch diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedGenericSignatureInstantiatesRecursiveWitness(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		local M = {}
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode<T>(data: string, witness: Type<T>): T
			return witness.decode(data)
		end
		return M
	`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local root = json.decode("{}", protocol.node_type())
		local id: string = root.id
		local wrong_id: number = root.id
		for _, child in ipairs(root.children) do
			local child_id: string = child.id
			local wrong_child_id: number = child.id
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("json", jsonMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want wrong root and child id diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
	}
}

func TestRequireCheckAndExportedGenericSignatureInstantiatesRecursiveUnionWitness(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type TextNode = {
			kind: "text",
			value: string,
		}
		type GroupNode = {
			kind: "group",
			children: {TreeNode},
		}
		type TreeNode = TextNode | GroupNode
		type RawRecord = {
			id: string,
			amount: number,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		local M = {}
		function M.raw_record_type(): Type<RawRecord>
			return {
				decode = function(raw: any): RawRecord
					return { id = tostring(raw), amount = 1 }
				end,
			}
		end
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		function M.tree_type(): Type<TreeNode>
			return {
				decode = function(raw: any): TreeNode
					return {
						kind = "group",
						children = {
							{
								kind = "text",
								value = tostring(raw),
							},
						},
					}
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode<T>(data: string, witness: Type<T>): T
			return witness.decode(data)
		end
		return M
	`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local record = json.decode("{}", protocol.raw_record_type())
		local id: string = record.id
		local root = json.decode("{}", protocol.node_type())
		local root_id: string = root.id
		local tree = json.decode("{}", protocol.tree_type())
		if tree.kind == "group" then
			local first = tree.children[1]
			if first and first.kind == "text" then
				local value: string = first.value
				local bad_value: number = first.value
			end
		end
		if tree.kind == "text" then
			local children = tree.children
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("json", jsonMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want recursive union witness mismatch diagnostics", checked.Diagnostics)
	}
	messages := make([]string, 0, len(checked.Diagnostics))
	for _, diag := range checked.Diagnostics {
		messages = append(messages, diag.Message)
		if diag.Code != diagnostics.CodeAssignmentType && diag.Code != diagnostics.CodeMissingMember {
			t.Fatalf("diagnostic code = %s, want assignment or member-read diagnostic", diag.Code)
		}
	}
	if !hasDiagnosticMessage(messages, "cannot assign first.value because it is string, not number") ||
		!hasDiagnosticMessage(messages, `has no member "children"`) {
		t.Fatalf("diagnostics = %#v, want first.value mismatch and text.children missing-member", messages)
	}
}

func TestRequireCheckAndExportedGenericSignatureSeedsCallbackParamFromRecursiveWitness(t *testing.T) {
	protocolMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		type Node = {
			id: string,
			children: {Node},
		}
		local M = {}
		function M.node_type(): Type<Node>
			return {
				decode = function(raw: any): Node
					return { id = tostring(raw), children = {} }
				end,
			}
		end
		return M
	`, "protocol", WithStdlib())
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	jsonMod := CheckAndExport(`
		type Type<T> = {
			decode: (any) -> T,
		}
		local M = {}
		function M.decode_map<T, U>(data: string, witness: Type<T>, fn: (T) -> U): U
			return fn(witness.decode(data))
		end
		return M
	`, "json")
	if len(jsonMod.Errors) != 0 {
		t.Fatalf("json module errors = %#v, want none", jsonMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local json = require("json")
		local label = json.decode_map("{}", protocol.node_type(), function(node)
			local node_id: string = node.id
			local bad_node_id: number = node.id
			return node_id
		end)
		local accepted: string = label
		local bad_label: number = label
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("json", jsonMod))
	if len(checked.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want bad_node_id and bad_label diagnostics", checked.Diagnostics)
	}
	for _, diag := range checked.Diagnostics {
		if diag.Code != diagnostics.CodeAssignmentType {
			t.Fatalf("diagnostic code = %s, want %s", diag.Code, diagnostics.CodeAssignmentType)
		}
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
	mod := CheckAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
		end
		return client
	`, "bedrock_client")
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
	mod := CheckAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
		end
		return client
	`, "bedrock_client")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	result := Check(`
		local bedrock_client = require("bedrock_client")
		local function helper(client, model_id)
			return client.invoke(model_id, {}, {})
		end
		local contract_args = nil :: any
		local model_id = contract_args.model
		helper(bedrock_client, model_id)
	`, WithStdlib(), WithModule("bedrock_client", mod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeDirectCallArgType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeDirectCallArgType)
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
			requireEvidenceMessage(t, result.Diagnostics[0], "no guard on this path proves mod[\"value\"] is non-nil")
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
	classRecord, ok := classField.Type.(*typ.Record)
	if !ok {
		t.Fatalf("Widget field type = %T %[1]v, want record", classField.Type)
	}
	requireFunctionField(t, classRecord, "new")
	requireFunctionField(t, classRecord, "label")
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
	if mismatch.Diagnostics[0].Code != diagnostics.CodeDirectCallResultAssignment {
		t.Fatalf("diagnostic code = %s, want %s", mismatch.Diagnostics[0].Code, diagnostics.CodeDirectCallResultAssignment)
	}

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
	if globalMismatch.Diagnostics[0].Code != diagnostics.CodeDirectCallResultAssignment {
		t.Fatalf("global diagnostic code = %s, want %s", globalMismatch.Diagnostics[0].Code, diagnostics.CodeDirectCallResultAssignment)
	}

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
	if result.Diagnostics[0].Code != diagnostics.CodeMissingMember {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeMissingMember)
	}
	requireEvidenceMessage(t, result.Diagnostics[0], "pkg.run has receiver type")
}

func TestTypedImportedMemberSignatureDoesNotSuppressOptionalReceiverDiagnostic(t *testing.T) {
	m := manifest.New("pkg")
	runType := typ.Func().Build()
	m.SetExport(typetable.NewRecord().Field("run", runType).Build())
	m.DefineFunctionSignature("pkg.run", signature.Function{Type: runType})

	result := Check(`
		local pkg: {run: () -> ()}? = require("pkg")
		pkg.run()
	`, WithStdlib(), WithManifest("pkg", m))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Code != diagnostics.CodeMissingMember {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeMissingMember)
	}
	requireEvidenceMessage(t, result.Diagnostics[0], "pkg.run has receiver type")
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
	requireDirectCallResultDiagnosticWithEvidence(t, mismatch, "typed imported provider member result")
	requireEvidenceMessage(t, mismatch.Diagnostics[0], "provider.meta returns")
	requireEvidenceMessage(t, mismatch.Diagnostics[0], "assignment target n requires number")
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
	if len(unknown.Diagnostics) != 0 {
		t.Fatalf("unknown diagnostics = %#v, want none without manifest precision", unknown.Diagnostics)
	}

	dynamic := Check(`
		local provider = require(module_name)
		local n: number = provider.meta()
	`, WithStdlib(), WithManifest("provider", m), WithGlobals("module_name"))
	if len(dynamic.Diagnostics) != 0 {
		t.Fatalf("dynamic diagnostics = %#v, want none without exact literal precision", dynamic.Diagnostics)
	}
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
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want none without exact manifest match", result.Diagnostics)
			}
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
		t.Fatalf("max shared placement depth = %d, want at least item -> child -> meta: %s\ncalls: %s", depth, placementSummary(root.Registry(), root.KeySpace(), exit), callOutcomeDebug(root))
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
			resultIDs := ""
			for i, result := range outcome.Results {
				if id, ok := valueIdentity(root.Registry(), result.Value); ok {
					if resultIDs != "" {
						resultIDs += ","
					}
					resultIDs += fmt.Sprintf("%d:%s", i, id)
				}
			}
			if resultIDs == "" {
				resultIDs = "none"
			}
			outcomeText = fmt.Sprintf("escapes=%d heap=%d results=[%s]", len(outcome.NormalReturnFacts.EscapeEvents), len(outcome.HeapTableObjects), resultIDs)
		}
		paths := ""
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
		if out != "" {
			out += "; "
		}
		out += fmt.Sprintf("p%d %s %s args=[%s]", point, signatureText, outcomeText, paths)
	}
	if out == "" {
		return "<no calls>"
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
