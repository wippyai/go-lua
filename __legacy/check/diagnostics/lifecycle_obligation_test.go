package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	lifecyclefx "github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestLifecycleResourceUnreleasedWarningIsDefaultAndEvidenceBacked(t *testing.T) {
	src := "local tx = {}\nbegin(tx)\n"
	result := runDiagnosticsResultFull(t, src, []string{"begin"}, lifecycleSignatureSource())
	if diags := ProduceWithConfig(result, Config{}); len(diags) != 1 || diags[0].Code != CodeResourceUnreleased || diags[0].Severity != diagnostic.SeverityWarning {
		t.Fatalf("default diagnostics = %#v, want one lifecycle warning", diagnosticMessages(diags))
	}

	diags := ProduceWithConfig(result, lifecycleDiagnosticsConfig())
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning", diagnosticMessages(diags))
	}
	d := diags[0]
	if d.Code != CodeResourceUnreleased || d.Severity != diagnostic.SeverityWarning {
		t.Fatalf("diagnostic = %#v, want lifecycle unreleased warning", d)
	}
	for _, want := range []string{
		"resource `tx` remains in transaction state `active` at function exit; expected `finished`",
	} {
		if !strings.Contains(d.Message, want) {
			t.Fatalf("message = %q, want %q", d.Message, want)
		}
	}
	requireEvidenceContains(t, d, "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends")
	requireEvidenceContains(t, d, "exit state still has `tx` in protocol transaction at `active`; no proof reaches `finished` or escapes ownership on every path")
	requireLabelContains(t, d, labelLifecycleAcquire)
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"main.lua": src},
		ShowSourceLabelRows: true,
	})
	for _, want := range []string{
		"warning[effect.lifecycle.unreleased]",
		"begin(tx)",
		"resource acquired",
		"missing proof",
		"help: Transition `tx` to `finished` or escape ownership on every return path.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", want, rendered)
		}
	}
}

func TestLifecycleResourceUnreleasedWarningIncludesPartialCloseEvidence(t *testing.T) {
	src := "local tx = {}\nbegin(tx)\nif flag then\n    finish(tx)\nend\n"
	result := runDiagnosticsResultFull(t, src, []string{"begin", "finish", "flag"}, lifecycleSignatureSource())

	diags := ProduceWithConfig(result, lifecycleDiagnosticsConfig())
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning", diagnosticMessages(diags))
	}
	d := diags[0]
	requireEvidenceContains(t, d, "this call transitions `tx` in protocol transaction from `active` to `finished` on a reachable path")
	requireEvidenceContains(t, d, "no proof reaches `finished` or escapes ownership on every path")
	requireLabelContains(t, d, labelLifecycleAcquire)
	requireLabelContains(t, d, labelLifecycleTransition)
}

func TestLifecycleResourceUnreleasedWarningIncludesPartialEscapeEvidence(t *testing.T) {
	src := "local tx = {}\nbegin(tx)\nif flag then\n    transfer(tx)\nend\n"
	result := runDiagnosticsResultFull(t, src, []string{"begin", "transfer", "flag"}, lifecycleSignatureSource())

	diags := ProduceWithConfig(result, lifecycleDiagnosticsConfig())
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one lifecycle warning", diagnosticMessages(diags))
	}
	d := diags[0]
	requireEvidenceContains(t, d, "this call escapes local ownership of `tx` in protocol transaction on a reachable path")
	requireEvidenceContains(t, d, "no proof reaches `finished` or escapes ownership on every path")
	requireLabelContains(t, d, labelLifecycleAcquire)
	requireLabelContains(t, d, labelLifecycleEscape)
}

func TestLifecycleResourceClosedOrEscapedOnEveryPathDoesNotWarn(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "closed",
			src:  "local tx = {}\nbegin(tx)\nfinish(tx)\n",
		},
		{
			name: "escaped",
			src:  "local tx = {}\nbegin(tx)\ntransfer(tx)\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runDiagnosticsResultFull(t, tc.src, []string{"begin", "finish", "transfer"}, lifecycleSignatureSource())
			if diags := ProduceWithConfig(result, lifecycleDiagnosticsConfig()); len(diags) != 0 {
				t.Fatalf("diagnostics = %#v, want no lifecycle warning", diagnosticMessages(diags))
			}
		})
	}
}

func TestLifecycleResourceWarningsDoNotUseOpenCloseNameHeuristics(t *testing.T) {
	t.Run("unannotated names do nothing", func(t *testing.T) {
		src := "local tx = {}\nopen(tx)\nclose(tx)\n"
		result := runDiagnosticsResultFull(t, src, []string{"open", "close"}, signaturelookup.Source{})
		if diags := ProduceWithConfig(result, lifecycleDiagnosticsConfig()); len(diags) != 0 {
			t.Fatalf("diagnostics = %#v, want no lifecycle warning from names alone", diagnosticMessages(diags))
		}
	})

	t.Run("non-resource-looking name with lifecycle fact warns", func(t *testing.T) {
		src := "local tx = {}\nweird(tx)\n"
		result := runDiagnosticsResultFull(t, src, []string{"weird"}, lifecycleSignatureSource())
		diags := ProduceWithConfig(result, lifecycleDiagnosticsConfig())
		if len(diags) != 1 || diags[0].Code != CodeResourceUnreleased {
			t.Fatalf("diagnostics = %#v, want lifecycle warning from fact-backed weird()", diagnosticMessages(diags))
		}
	})
}

func lifecycleDiagnosticsConfig() Config {
	return Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeResourceUnreleased: diagnostic.Enable(),
	}}}
}

func lifecycleSignatureSource() signaturelookup.Source {
	m := manifest.New("lifecycle")
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    typestate.Protocol("transaction"),
		States:      []typestate.State{typestate.State("active"), typestate.State("finished")},
		FinalStates: []typestate.State{typestate.State("finished")},
		Transitions: []typestate.TransitionDecl{{
			From: typestate.State("active"),
			To:   typestate.State("finished"),
		}},
	}); err != nil {
		panic(err)
	}
	acquire := lifecyclefx.Acquire{
		Target:   effect.ParamRef{Index: 0},
		Protocol: typestate.Protocol("transaction"),
		State:    typestate.State("active"),
		Obligation: typestate.Obligation{
			Final: typestate.State("finished"),
		},
	}
	transition := lifecyclefx.Transition{
		Target:   effect.ParamRef{Index: 0},
		Protocol: typestate.Protocol("transaction"),
		From:     typestate.State("active"),
		To:       typestate.State("finished"),
	}
	escape := lifecyclefx.Escape{
		Target:   effect.ParamRef{Index: 0},
		Protocol: typestate.Protocol("transaction"),
	}
	for _, name := range []string{"begin", "weird"} {
		m.DefineFunctionSignature(name, signature.Function{
			Type: typ.Func().
				Param("tx", typ.Any).
				Build(),
			Effect: effect.Row{Labels: []effect.Label{acquire}},
		})
	}
	m.DefineFunctionSignature("finish", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Row{Labels: []effect.Label{transition}},
	})
	m.DefineFunctionSignature("transfer", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Row{Labels: []effect.Label{escape}},
	})
	return signaturelookup.Source{Manifests: []*manifest.Manifest{m}}
}

func requireEvidenceContains(t *testing.T, d diagnostic.Diagnostic, want string) {
	t.Helper()
	for _, evidence := range d.Explanation.Evidence() {
		if strings.Contains(evidence.Message, want) {
			return
		}
	}
	t.Fatalf("evidence = %#v, want message containing %q", d.Explanation.Evidence(), want)
}

func requireLabelContains(t *testing.T, d diagnostic.Diagnostic, want string) {
	t.Helper()
	for _, label := range d.Labels {
		if strings.Contains(label.Message, want) {
			return
		}
	}
	t.Fatalf("labels = %#v, want label containing %q", d.Labels, want)
}
