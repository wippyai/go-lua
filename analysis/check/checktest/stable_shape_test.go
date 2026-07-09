package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
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
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireShapeFieldSubtype(t, root, fact.Shape, "port", typ.Number)
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
	requireShapeFieldSubtype(t, root, fact.Shape, "host", typ.String)
	requireShapeFieldSubtype(t, root, fact.Shape, "port", typ.Number)
}

func TestStableShapeRejectsAliasMutationAfterRead(t *testing.T) {
	result := Check(`
local cfg = {}
cfg.host = "x"
local alias = cfg
local host = cfg.host
alias.port = 80
`)
	requireNoDiagnostics(t, result.Diagnostics)
	root := requireRootResult(t, result)
	occ := requireStaticRead(t, root, "cfg.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after reachable alias mutation", fact)
	}
}

func TestStableShapeRejectsConditionalFieldAdd(t *testing.T) {
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
	occ := requireStaticRead(t, root, "cfg.host")
	if fact, ok := root.StableShapeForStaticMemberRead(occ); ok {
		t.Fatalf("stable shape = %#v, want no fact after conditional field add", fact)
	}
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
