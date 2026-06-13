package checktest

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	if result.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", result.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
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
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for local f shadowing manifest f", result.Diagnostics)
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
	if len(mismatch.Diagnostics) != 1 {
		t.Fatalf("mismatch diagnostics = %d, want 1: %#v", len(mismatch.Diagnostics), mismatch.Diagnostics)
	}
	if mismatch.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", mismatch.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
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
