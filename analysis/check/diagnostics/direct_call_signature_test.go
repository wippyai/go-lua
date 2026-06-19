package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestDirectCallReportsManifestSignatureWrongArgumentType(t *testing.T) {
	src := `imported(42, 1)`
	diags := runDiagnosticsWithImportedSignature(t, src)
	d := requireDirectCallDiagnostic(t, diags, CodeDirectCallArgType)
	if d.Position.Line != 1 || d.Position.Column != 10 || d.Position.EndColumn != 11 {
		t.Fatalf("position = %#v, want exact span of argument 42", d.Position)
	}
	if d.Span.StartLine != 1 || d.Span.StartCol != 10 || d.Span.EndCol != 11 {
		t.Fatalf("span = %#v, want exact span of argument 42", d.Span)
	}
	if !diagnosticHasLabel(d, "argument value") {
		t.Fatalf("labels = %#v, want argument value label", d.Labels)
	}
	if got := d.Labels[0].Span; got.StartLine != d.Span.StartLine || got.StartCol != d.Span.StartCol || got.EndCol != d.Span.EndCol {
		t.Fatalf("argument label span = %#v, want primary span %#v", got, d.Span)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) == 0 || evidence[0].Span.StartLine != d.Span.StartLine || evidence[0].Span.StartCol != d.Span.StartCol || evidence[0].Span.EndCol != d.Span.EndCol {
		t.Fatalf("first evidence span = %#v, want primary argument span %#v", evidence, d.Span)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "argument 1 has literal value 42") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "imported parameter 1 expects string") {
		t.Fatalf("evidence = %#v, want argument value and imported signature declaration", d.Explanation.Evidence())
	}
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"main.lua": src},
		ShowSourceLabelRows: true,
	})
	requireRenderedContains(t, rendered,
		" --> main.lua:1:10",
		"1 | imported(42, 1)",
		"  |          ↑ argument value",
		"1. proven: argument 1 has literal value 42",
		"2. claimed: imported parameter 1 expects string",
	)
	if strings.Contains(rendered, "  | ^") {
		t.Fatalf("rendered diagnostic should not add a whole-call arrow before the argument marker:\n%s", rendered)
	}
}

func TestDirectCallReportsManifestSignatureTooFewArgs(t *testing.T) {
	src := `imported("ok")`
	diags := runDiagnosticsWithImportedSignature(t, src)
	d := requireDirectCallDiagnostic(t, diags, CodeDirectCallTooFewArgs)
	if d.Message != "imported expects 2 arguments, got 1" {
		t.Fatalf("message = %q, want precise arity mismatch", d.Message)
	}
	if !diagnosticHasLabel(d, labelCallExpression) {
		t.Fatalf("labels = %#v, want call-expression focus label", d.Labels)
	}
	if !diagnosticEvidenceContains(d.Explanation.Evidence(), "passes 1 argument") ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "imported declares 2 parameters") {
		t.Fatalf("evidence = %#v, want call arity and imported signature declaration", d.Explanation.Evidence())
	}
	if !strings.Contains(d.Help, "Pass the missing required arguments") {
		t.Fatalf("help = %q, want missing-argument repair", d.Help)
	}
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"main.lua": src},
		ShowSourceLabelRows: true,
	})
	requireRenderedContains(t, rendered,
		"error[type.call.direct.too_few_args]: imported expects 2 arguments, got 1",
		"1 | imported(\"ok\")",
		"  | ↑ call expression",
		"1. proven: call to imported passes 1 argument",
		"2. claimed: imported declares 2 parameters",
		"help: Pass the missing required arguments",
	)
}

func TestDirectCallReportsManifestSignatureTooManyArgs(t *testing.T) {
	src := `imported("ok", 42, true)`
	diags := runDiagnosticsWithImportedSignature(t, src)
	d := requireDirectCallDiagnostic(t, diags, CodeDirectCallTooManyArgs)
	if d.Message != "imported expects 2 arguments, got 3" {
		t.Fatalf("message = %q, want precise arity mismatch", d.Message)
	}
	if len(d.Labels) != 1 || d.Labels[0].Message != labelExtraArgument {
		t.Fatalf("labels = %#v, want extra argument label", d.Labels)
	}
	if got := d.Labels[0].Span; got.StartLine != 1 || got.StartCol != 20 || got.EndCol != 23 {
		t.Fatalf("extra argument span = %#v, want exact span for true", got)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 2 ||
		evidence[0].Kind != diagnostic.EvidenceAbstractFact ||
		evidence[0].Trust != diagnostic.TrustProven ||
		evidence[1].Kind != diagnostic.EvidenceUserAssertion ||
		evidence[1].Trust != diagnostic.TrustClaimed {
		t.Fatalf("evidence = %#v, want proven call count and claimed signature count", evidence)
	}
	if !diagnosticEvidenceContains(evidence, "call to imported passes 3 arguments") ||
		!diagnosticEvidenceContains(evidence, "imported declares 2 parameters") {
		t.Fatalf("evidence = %#v, want call and declaration evidence", evidence)
	}
	if !strings.Contains(d.Help, "Remove the extra argument") {
		t.Fatalf("help = %q, want extra-argument repair", d.Help)
	}
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"main.lua": src},
		ShowSourceLabelRows: true,
	})
	requireRenderedContains(t, rendered,
		"error[type.call.direct.too_many_args]: imported expects 2 arguments, got 3",
		"1 | imported(\"ok\", 42, true)",
		"  |                    ↑ extra argument",
		"1. proven: call to imported passes 3 arguments",
		"2. claimed: imported declares 2 parameters",
		"help: Remove the extra argument",
	)
	if strings.Contains(rendered, "  | ^") {
		t.Fatalf("rendered diagnostic should not add a whole-call arrow before the extra argument marker:\n%s", rendered)
	}
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
	if d := diags[0]; !diagnosticHasLabel(d, labelUnknownValue) ||
		!diagnosticEvidenceContains(d.Explanation.Evidence(), "no value named imported is declared") {
		t.Fatalf("diagnostic = %#v, want unresolved imported missing-declaration proof with unknown-value label", d)
	}
	evidence := diags[0].Explanation.Evidence()
	if len(evidence) != 1 || evidence[0].Kind != diagnostic.EvidenceAbstractFact || evidence[0].Trust != diagnostic.TrustProven {
		t.Fatalf("evidence = %#v, want one proven lookup fact", evidence)
	}
	if !strings.Contains(diags[0].Help, "Declare the value") ||
		!strings.Contains(diags[0].Help, "configured globals") {
		t.Fatalf("help = %q, want actionable imported-global resolution help", diags[0].Help)
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

func requireDirectCallDiagnostic(t *testing.T, diags []diagnostic.Diagnostic, code diagnostic.Code) diagnostic.Diagnostic {
	t.Helper()
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != code || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s, want %s/%s", d.Code, d.Severity, code, diagnostic.SeverityError)
	}
	return diags[0]
}

func requireRenderedContains(t *testing.T, rendered string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", fragment, rendered)
		}
	}
}
