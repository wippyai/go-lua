package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestStableShapePromotesIncrementallyBuiltPlainTable(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
cfg.port = 80
local host = cfg.host
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "cfg.host")
	requireShapeTier(t, fact, body.StableShapeTierStableAfterPoint)
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireShapeFieldSubtype(t, root, fact.Shape, "port", typ.Number)
}

func TestPrefixStableLicensesReadBeforeLaterExtension(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
local host = cfg.host
cfg.port = 80
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "cfg.host")
	requireShapeTier(t, fact, body.StableShapeTierPrefixStable)
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireNoShapeField(t, fact.Shape, "port")
}

func TestStableShapeCrossesManifestFunctionReturnBoundary(t *testing.T) {
	builder := CheckAndExport(`
local M = {}

function M.make()
  local cfg = {}
  cfg.host = "x"
  cfg.port = 80
  return cfg
end

return M
`, "builder")
	requireNoDiagnostics(t, builder.Errors)
	requireStableAllocationTemplate(t, builder)

	result := Check(`
local builder = require("builder")
local cfg = builder.make()
local host = cfg.host
`, WithModule("builder", builder))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "cfg.host")
	requireShapeTier(t, fact, body.StableShapeTierStable)
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireShapeFieldSubtype(t, root, fact.Shape, "port", typ.Number)
}

func TestPrefixStableShapeCrossesManifestIdentityReturnFlowBoundary(t *testing.T) {
	helpers := CheckAndExport(`
local M = {}

function M.id(value: table): table
  return value
end

return M
`, "helpers")
	requireNoDiagnostics(t, helpers.Errors)
	requireManifestReturnFlow(t, helpers, "helpers.id", signature.ReturnFlowParam)

	result := Check(`
local helpers = require("helpers")
local cfg = {}
cfg.host = "x"
local same = helpers.id(cfg)
local host = same.host
cfg.port = 80
`, WithModule("helpers", helpers))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "same.host")
	requireShapeTier(t, fact, body.StableShapeTierPrefixStable)
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireNoShapeField(t, fact.Shape, "port")
}

func TestManifestReturnFlowDefaultOrDoesNotPreserveShape(t *testing.T) {
	helpers := CheckAndExport(`
local M = {}

function M.default_or(value: table?, fallback: table): table
  return value or fallback
end

return M
`, "helpers")
	requireNoDiagnostics(t, helpers.Errors)
	requireManifestNoReturnFlow(t, helpers, "helpers.default_or")

	result := Check(`
local helpers = require("helpers")
local cfg = {}
cfg.host = "x"
local fallback = {}
fallback.other = 1
local mixed = helpers.default_or(cfg, fallback)
local host = mixed.host
`, WithModule("helpers", helpers))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "mixed.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact for mixed default-or return", fact)
	}
}

func TestManifestReturnFlowGetterPreservesMemberShape(t *testing.T) {
	helpers := CheckAndExport(`
type Row = {
  meta: table,
}

local M = {}

function M.get_meta(row: Row): table
  return row.meta
end

return M
`, "helpers")
	requireNoDiagnostics(t, helpers.Errors)
	requireManifestReturnFlow(t, helpers, "helpers.get_meta", signature.ReturnFlowParamMember)

	result := Check(`
local helpers = require("helpers")
local row = {}
local source_meta = {}
source_meta.route = "a"
row.meta = source_meta
local meta = helpers.get_meta(row)
local route: string = meta.route
`, WithModule("helpers", helpers))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "meta.route")
	requireShapeFieldSubtype(t, root, fact.Shape, "route", typ.String)
}

func TestManifestReturnFlowMutatingHelperDegradesShape(t *testing.T) {
	helpers := CheckAndExport(`
local M = {}

function M.touch(value: table): table
  value.late = 1
  return value
end

return M
`, "helpers")
	requireNoDiagnostics(t, helpers.Errors)
	requireManifestReturnFlow(t, helpers, "helpers.touch", signature.ReturnFlowParam)

	result := Check(`
local helpers = require("helpers")
local cfg = {}
cfg.host = "x"
local touched = helpers.touch(cfg)
local host = touched.host
`, WithModule("helpers", helpers))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "touched.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after mutating return helper", fact)
	}
}

func TestManifestReturnFlowConditionalStoreDegradesShape(t *testing.T) {
	helpers := CheckAndExport(`
local M = {}
local sink = {}

function M.maybe_store(value: table, flag: boolean): table
  if flag then
    sink.saved = value
  end
  return value
end

return M
`, "helpers")
	requireNoDiagnostics(t, helpers.Errors)
	requireManifestReturnFlow(t, helpers, "helpers.maybe_store", signature.ReturnFlowParam)

	result := Check(`
local helpers = require("helpers")
local cfg = {}
cfg.host = "x"
local maybe = helpers.maybe_store(cfg, false)
local host = maybe.host
`, WithModule("helpers", helpers))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "maybe.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after conditional param store", fact)
	}
}

func TestPrefixStableSurvivesKnownAliasExtensionAfterRead(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
local alias = cfg
local host = cfg.host
alias.port = 80
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "cfg.host")
	requireShapeTier(t, fact, body.StableShapeTierPrefixStable)
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireNoShapeField(t, fact.Shape, "port")
}

func TestPrefixStableKeepsRequiredPrefixAcrossConditionalAdd(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
if unknown then
  cfg.port = 80
end
local host = cfg.host
`, WithGlobals("unknown"))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "cfg.host")
	requireShapeTier(t, fact, body.StableShapeTierPrefixStable)
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireNoShapeField(t, fact.Shape, "port")
}

func TestStableShapeRejectsMethodCallThatWritesSelf(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
function cfg:add_port()
  self.port = 80
end
local host = cfg.host
cfg:add_port()
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "cfg.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact before reachable self mutation", fact)
	}
}

func TestPrefixStableRejectsUnknownWriter(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
external(cfg)
local host = cfg.host
`, WithGlobals("external"))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "cfg.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after unknown writer", fact)
	}
}

func TestPrefixStableRejectsDelete(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
cfg.host = nil
local host = cfg.host
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "cfg.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after field delete", fact)
	}
}

func TestPrefixStableRejectsRetypeThroughAlias(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
local alias = cfg
alias.host = unknown_value
local host = cfg.host
`, WithGlobals("unknown_value"))
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "cfg.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after alias retype", fact)
	}
}

func TestPrefixStableRejectsDynamicKeyWrite(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
local key = "port"
cfg[key] = 80
local host = cfg.host
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "cfg.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after dynamic-key write", fact)
	}
}

func TestPrefixStableLoopAddDoesNotLicenseLoopField(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
while unknown do
  cfg.port = 80
end
local port = cfg.port
`, WithGlobals("unknown"))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "cfg.port")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact for loop-added field", fact)
	}
}

func TestModuleReturnSnapshotDoesNotInventLaterFields(t *testing.T) {
	builder := CheckAndExport(`
local M = {}
M.ready = true
return M
`, "service")
	requireNoDiagnostics(t, builder.Errors)

	result := Check(`
local service = require("service")
local ready = service.ready
`, WithModule("service", builder))
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	fact := requireStableReadShape(t, root, "service.ready")
	requireShapeFieldSubtype(t, root, fact.Shape, "ready", typ.Boolean)
	requireNoShapeField(t, fact.Shape, "late")
}

func TestStableShapePromotesMethodTablePattern(t *testing.T) {
	result := Check(`
local M = {}
function M.foo()
  return "ok"
end
return M
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	if len(root.ReturnPoints()) == 0 {
		t.Fatal("missing return point")
	}
	sources, ok := root.ReturnValueSources(root.ReturnPoints()[0])
	if !ok || len(sources) == 0 {
		t.Fatal("missing return sources")
	}
	if !root.SourceHasStableShapeBeforeBoundary(root.ReturnPoints()[0], sources[0]) {
		t.Fatal("method table return did not earn stable shape")
	}
}

func requireRootResult(t *testing.T, result Result) *body.Result {
	t.Helper()
	root := result.RootResult()
	if root == nil {
		t.Fatal("missing root result")
	}
	return root
}

func requireNoDiagnostics(t *testing.T, diagnostics []diagnostic.Diagnostic) {
	t.Helper()
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %d, want 0: %#v", len(diagnostics), diagnostics)
	}
}

func requireStaticRead(t *testing.T, result *body.Result, label string) body.StaticMemberReadOccurrence {
	t.Helper()
	var found body.StaticMemberReadOccurrence
	count := 0
	result.ForEachStaticMemberReadOccurrence(func(occ body.StaticMemberReadOccurrence) bool {
		if occ.ReadLabel == label {
			found = occ
			count++
		}
		return true
	})
	if count != 1 {
		t.Fatalf("read %q count = %d, want 1", label, count)
	}
	return found
}

func requireStableReadShape(t *testing.T, result *body.Result, label string) body.StableShapeFact {
	t.Helper()
	occ := requireStaticRead(t, result, label)
	fact, ok := result.StableShapeForStaticMemberRead(occ)
	if !ok {
		t.Fatalf("read %q has no stable shape", label)
	}
	return fact
}

func requireShapeTier(t *testing.T, fact body.StableShapeFact, want body.StableShapeTier) {
	t.Helper()
	if fact.Tier != want {
		t.Fatalf("shape tier = %s, want %s", fact.Tier, want)
	}
}

func requireShapeFieldSubtype(t *testing.T, result *body.Result, shape typ.Type, name string, want typ.Type) {
	t.Helper()
	got, ok := body.TypeField(shape, name)
	if !ok {
		t.Fatalf("shape %v missing field %s", shape, name)
	}
	if !result.IsSubtype(got, want) {
		t.Fatalf("shape field %s = %v, want subtype of %v", name, got, want)
	}
}

func requireNoShapeField(t *testing.T, shape typ.Type, name string) {
	t.Helper()
	if got, ok := body.TypeField(shape, name); ok {
		t.Fatalf("shape field %s = %v, want absent", name, got)
	}
}

func requireStableAllocationTemplate(t *testing.T, mod *ModuleResult) {
	t.Helper()
	if mod == nil || mod.Manifest == nil {
		t.Fatal("missing module manifest")
	}
	for _, sig := range mod.Manifest.FunctionSignatures {
		if sig.OperationalEffects == nil {
			continue
		}
		for _, template := range sig.OperationalEffects.ReturnAllocationTemplates {
			for _, object := range template.Objects {
				if object.ID == template.Root && object.StableShape {
					return
				}
			}
		}
	}
	t.Fatal("manifest has no stable return allocation template")
}

func requireManifestReturnFlow(t *testing.T, mod *ModuleResult, name string, kind signature.ReturnFlowKind) {
	t.Helper()
	if mod == nil || mod.Manifest == nil {
		t.Fatal("missing module manifest")
	}
	sig, ok := mod.Manifest.FunctionSignatures[name]
	if !ok {
		t.Fatalf("missing %s function signature: %#v", name, mod.Manifest.FunctionSignatures)
	}
	if sig.OperationalEffects == nil {
		t.Fatalf("%s operational effects = nil", name)
	}
	for _, flow := range sig.OperationalEffects.ReturnFlows {
		if flow.Kind == kind {
			return
		}
	}
	t.Fatalf("%s return flows = %#v, want kind %d", name, sig.OperationalEffects.ReturnFlows, kind)
}

func requireManifestNoReturnFlow(t *testing.T, mod *ModuleResult, name string) {
	t.Helper()
	if mod == nil || mod.Manifest == nil {
		t.Fatal("missing module manifest")
	}
	sig, ok := mod.Manifest.FunctionSignatures[name]
	if !ok {
		t.Fatalf("missing %s function signature: %#v", name, mod.Manifest.FunctionSignatures)
	}
	if sig.OperationalEffects != nil && len(sig.OperationalEffects.ReturnFlows) != 0 {
		t.Fatalf("%s return flows = %#v, want none", name, sig.OperationalEffects.ReturnFlows)
	}
}
