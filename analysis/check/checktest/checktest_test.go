package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/module/manifest"
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

	mismatch := Check(`local x: string = f()`, WithManifest("test", m))
	if len(mismatch.Diagnostics) != 1 {
		t.Fatalf("mismatch diagnostics = %d, want 1: %#v", len(mismatch.Diagnostics), mismatch.Diagnostics)
	}
	if mismatch.Diagnostics[0].Code != diagnostics.CodeDirectCallResultAssignment {
		t.Fatalf("diagnostic code = %s, want %s", mismatch.Diagnostics[0].Code, diagnostics.CodeDirectCallResultAssignment)
	}

	matching := Check(`local x: number = f()`, WithManifest("test", m))
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
	`, WithManifest("test", m))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for local f shadowing manifest f", result.Diagnostics)
	}
}

func TestWithManifestResolvesDottedGlobalStaticCalleePathOnly(t *testing.T) {
	m := manifest.New("test")
	m.DefineFunctionSignature("pkg.make", signature.Function{
		Type: typ.Func().Returns(typ.Number).Build(),
	})

	globalMismatch := Check(`local x: string = pkg.make()`, WithManifest("test", m))
	if len(globalMismatch.Diagnostics) != 1 {
		t.Fatalf("global diagnostics = %d, want 1: %#v", len(globalMismatch.Diagnostics), globalMismatch.Diagnostics)
	}
	if globalMismatch.Diagnostics[0].Code != diagnostics.CodeDirectCallResultAssignment {
		t.Fatalf("global diagnostic code = %s, want %s", globalMismatch.Diagnostics[0].Code, diagnostics.CodeDirectCallResultAssignment)
	}

	localRoot := Check(`
		local pkg = {}
		local x: string = pkg.make()
	`, WithManifest("test", m))
	if len(localRoot.Diagnostics) != 0 {
		t.Fatalf("local-root diagnostics = %#v, want none", localRoot.Diagnostics)
	}
}
