package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDirectCallReportsManifestSignatureWrongArgumentType(t *testing.T) {
	diags := runDiagnosticsWithImportedSignature(t, `imported(42, 1)`)
	requireDirectCallDiagnostic(t, diags, CodeDirectCallArgType)
}

func TestDirectCallReportsManifestSignatureTooFewArgs(t *testing.T) {
	diags := runDiagnosticsWithImportedSignature(t, `imported("ok")`)
	requireDirectCallDiagnostic(t, diags, CodeDirectCallTooFewArgs)
}

func TestDirectCallAcceptsManifestSignatureArguments(t *testing.T) {
	diags := runDiagnosticsWithImportedSignature(t, `imported("ok", 42)`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for matching manifest signature call", diags)
	}
}

func TestDirectCallManifestSignatureDoesNotOverrideLocalShadow(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
		local function imported()
		end
		imported()
	`, directCallSignatureSource())
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for local function shadowing manifest signature", diags)
	}
}

func TestDirectCallManifestSignatureRequiresExplicitGlobal(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `imported(42, 1)`, directCallSignatureSource())
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want unresolved imported only: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeUnresolvedValueReference {
		t.Fatalf("diagnostic code = %s, want %s", diags[0].Code, CodeUnresolvedValueReference)
	}
}

func runDiagnosticsWithImportedSignature(t *testing.T, src string) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsFull(t, src, []string{"test", "type", "value", "imported"}, directCallSignatureSource())
}

func directCallSignatureSource() signaturelookup.Source {
	m := manifest.New("test")
	m.DefineFunctionSignature("imported", signature.Function{
		Type: typ.Func().
			Param("name", typ.String).
			Param("count", typ.Number).
			Build(),
	})
	return signaturelookup.Source{
		Manifests: []*manifest.Manifest{m},
	}
}

func requireDirectCallDiagnostic(t *testing.T, diags []diagnostic.Diagnostic, code diagnostic.Code) {
	t.Helper()
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != code || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s, want %s/%s", d.Code, d.Severity, code, diagnostic.SeverityError)
	}
}
