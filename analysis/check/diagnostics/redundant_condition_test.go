package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRedundantConditionWarningIsOptInAndEvidenceBacked(t *testing.T) {
	result := runDiagnosticsResult(t, `
local value = test
if value then
	if value then
		return value
	end
end
`)
	if diags := ProduceWithConfig(result, Config{}); len(diags) != 0 {
		t.Fatalf("default diagnostics = %#v, want redundant-condition disabled by default", diags)
	}

	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one redundant-condition warning", diags)
	}
	d := diags[0]
	if d.Code != CodeRedundantCondition || d.Severity != diagnostic.SeverityWarning {
		t.Fatalf("diagnostic code/severity = %s/%s, want %s/warning", d.Code, d.Severity, CodeRedundantCondition)
	}
	if !strings.Contains(d.Message, "always true") {
		t.Fatalf("diagnostic = %#v, want always-true redundant condition", d)
	}
	if !diagnosticHasLabel(d, labelConditionCheck) {
		t.Fatalf("labels = %#v, want condition-check focus label", d.Labels)
	}
	if !diagnosticHasLabel(d, labelProvingGuard) {
		t.Fatalf("labels = %#v, want proving-guard cause label", d.Labels)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 3 {
		t.Fatalf("evidence = %#v, want condition, incoming proof, and stability proof", evidence)
	}
	if !strings.Contains(evidence[1].Message, "truthy") ||
		!strings.Contains(evidence[2].Message, "value is unchanged between the prior guard and this check") {
		t.Fatalf("evidence = %#v, want guard proof and stability evidence", evidence)
	}
	if !evidence[1].Span.Valid() || evidence[1].Span.StartLine >= d.Span.StartLine {
		t.Fatalf("proof evidence span = %#v, want earlier prior guard before condition span %#v", evidence[1].Span, d.Span)
	}
}

func TestRedundantConditionWarningRendersTruthyGuardTrace(t *testing.T) {
	src := strings.TrimLeft(`
local value = test
if value then
    if value then
        return value
    end
end
`, "\n")
	diags := ProduceWithConfig(runDiagnosticsResult(t, src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one redundant-condition warning", diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.condition.redundant]: condition is always true here
 --> diagnostics_test.lua:3:8
  |
3 |     if value then
  |        ↑ current check

because:
  1. proven: current check: value is checked as truthy
  2. proven: prior guard established value is truthy
 --> diagnostics_test.lua:2:4
  |
  |    ↓ prior guard
2 | if value then
  3. proven: value is unchanged between the prior guard and this check

help: Remove this repeated check, or move any needed work into the branch already guarded above.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestRedundantConditionWarningRendersUnreachableGuardTrace(t *testing.T) {
	src := strings.TrimLeft(`
local value = nil
if value == nil then
    if value ~= nil then
        return value
    end
end
`, "\n")
	diags := ProduceWithConfig(runDiagnosticsResult(t, src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one redundant-condition warning", diags)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.condition.redundant]: condition is always false here
 --> diagnostics_test.lua:3:8
  |
3 |     if value ~= nil then
  |        ↑ current check

because:
  1. proven: current check: value ~= nil
  2. proven: prior guard established value is nil
 --> diagnostics_test.lua:2:4
  |
  |    ↓ prior guard
2 | if value == nil then
  3. proven: value is unchanged between the prior guard and this check

help: Remove this unreachable branch, or change the prior guard if this path should still run.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestRedundantConditionWarningDistinguishesNilCheckEvidence(t *testing.T) {
	cases := []struct {
		name         string
		src          string
		wantMessage  string
		wantCheck    string
		wantProof    string
		wantBranch   string
		rejectCheck  string
		rejectBranch string
	}{
		{
			name: "known nil makes nil check true",
			src: `
local value = nil
if value == nil then
	if value == nil then
		return value
	end
end
`,
			wantMessage:  "always true",
			wantCheck:    "current check: value == nil",
			wantProof:    "prior guard established value is nil",
			wantBranch:   "value is unchanged between the prior guard and this check",
			rejectCheck:  "current check: value ~= nil",
			rejectBranch: "branch is unreachable",
		},
		{
			name: "known nil makes non-nil check false",
			src: `
local value = nil
if value == nil then
	if value ~= nil then
		return value
	end
end
`,
			wantMessage:  "always false",
			wantCheck:    "current check: value ~= nil",
			wantProof:    "prior guard established value is nil",
			wantBranch:   "value is unchanged between the prior guard and this check",
			rejectCheck:  "current check: value == nil",
			rejectBranch: "branch is unreachable",
		},
		{
			name: "known non-nil makes nil check false",
			src: `
local value = test
if value ~= nil then
	if value == nil then
		return value
	end
end
`,
			wantMessage:  "always false",
			wantCheck:    "current check: value == nil",
			wantProof:    "prior guard established value is not nil",
			wantBranch:   "value is unchanged between the prior guard and this check",
			rejectCheck:  "current check: value ~= nil",
			rejectBranch: "branch is unreachable",
		},
		{
			name: "known non-nil makes non-nil check true",
			src: `
local value = test
if value ~= nil then
	if value ~= nil then
		return value
	end
end
`,
			wantMessage:  "always true",
			wantCheck:    "current check: value ~= nil",
			wantProof:    "prior guard established value is not nil",
			wantBranch:   "value is unchanged between the prior guard and this check",
			rejectCheck:  "current check: value == nil",
			rejectBranch: "branch is unreachable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := ProduceWithConfig(runDiagnosticsResult(t, tc.src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
				CodeRedundantCondition: diagnostic.Enable(),
			}}})
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %#v, want one redundant-condition warning", diags)
			}
			d := diags[0]
			if d.Code != CodeRedundantCondition || !strings.Contains(d.Message, tc.wantMessage) {
				t.Fatalf("diagnostic = %#v, want %s redundant-condition warning", d, tc.wantMessage)
			}
			explanation := d.Explanation.String()
			for _, want := range []string{tc.wantCheck, tc.wantProof, tc.wantBranch} {
				if !strings.Contains(explanation, want) {
					t.Fatalf("explanation = %q, want %q", explanation, want)
				}
			}
			for _, reject := range []string{tc.rejectCheck, tc.rejectBranch, "checked for nil", "checked for non-nil", "CFG"} {
				if strings.Contains(explanation, reject) {
					t.Fatalf("explanation = %q, should not contain %q", explanation, reject)
				}
			}
		})
	}
}

func TestRedundantConditionWarningDistinguishesRuntimeNilTypeEvidence(t *testing.T) {
	cases := []struct {
		name         string
		src          string
		wantMessage  string
		wantCheck    string
		wantProof    string
		wantBranch   string
		rejectCheck  string
		rejectBranch string
	}{
		{
			name: "known nil makes runtime type-not-nil check false",
			src: `
local value = nil
if value == nil then
	if type(value) ~= "nil" then
		return value
	end
end
`,
			wantMessage:  "always false",
			wantCheck:    `current check: type(value) is not "nil"`,
			wantProof:    "prior guard established value is nil",
			wantBranch:   "value is unchanged between the prior guard and this check",
			rejectCheck:  `current check: type(value) is "nil"`,
			rejectBranch: "branch is unreachable",
		},
		{
			name: "known non-nil makes runtime type-nil check false",
			src: `
local value = test
if value ~= nil then
	if type(value) == "nil" then
		return value
	end
end
`,
			wantMessage:  "always false",
			wantCheck:    `current check: type(value) is "nil"`,
			wantProof:    "prior guard established value is not nil",
			wantBranch:   "value is unchanged between the prior guard and this check",
			rejectCheck:  `current check: type(value) is not "nil"`,
			rejectBranch: "branch is unreachable",
		},
		{
			name: "runtime type-not-nil guard makes repeated type-not-nil check true",
			src: `
local value = test
if type(value) ~= "nil" then
	if type(value) ~= "nil" then
		return value
	end
end
`,
			wantMessage:  "always true",
			wantCheck:    `current check: type(value) is not "nil"`,
			wantProof:    "prior guard established value is not nil",
			wantBranch:   "value is unchanged between the prior guard and this check",
			rejectCheck:  `current check: type(value) is "nil"`,
			rejectBranch: "branch is unreachable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := ProduceWithConfig(runDiagnosticsResult(t, tc.src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
				CodeRedundantCondition: diagnostic.Enable(),
			}}})
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %#v, want one redundant-condition warning", diags)
			}
			d := diags[0]
			if d.Code != CodeRedundantCondition || !strings.Contains(d.Message, tc.wantMessage) {
				t.Fatalf("diagnostic = %#v, want %s redundant-condition warning", d, tc.wantMessage)
			}
			explanation := d.Explanation.String()
			for _, want := range []string{tc.wantCheck, tc.wantProof, tc.wantBranch} {
				if !strings.Contains(explanation, want) {
					t.Fatalf("explanation = %q, want %q", explanation, want)
				}
			}
			for _, reject := range []string{tc.rejectCheck, tc.rejectBranch, "CFG"} {
				if strings.Contains(explanation, reject) {
					t.Fatalf("explanation = %q, should not contain %q", explanation, reject)
				}
			}
		})
	}
}

func TestRedundantConditionWarningReportsRepeatedGuardShapes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "truthy guard makes later truthy check false in else arm",
			src: `
local value = test
if value then
else
	if value then
		return value
	end
end
`,
			want: "always false",
		},
		{
			name: "nil guard makes not-nil check impossible",
			src: `
local value = nil
if value == nil then
	if value ~= nil then
		return value
	end
end
`,
			want: "always false",
		},
		{
			name: "not-nil guard makes nil check impossible",
			src: `
local value = test
if value ~= nil then
	if value == nil then
		return value
	end
end
`,
			want: "always false",
		},
		{
			name: "runtime type guard makes different type-not check redundant",
			src: `
local value = "x"
if type(value) == "string" then
	if type(value) ~= "number" then
		return value
	end
end
`,
			want: "always true",
		},
		{
			name: "runtime non-boolean type guard proves truthiness",
			src: `
local value = test
if type(value) == "string" then
	if value then
		return value
	end
end
`,
			want: "always true",
		},
		{
			name: "runtime type-not-nil guard makes nil check impossible",
			src: `
local value = test
if type(value) ~= "nil" then
	if value == nil then
		return value
	end
end
`,
			want: "always false",
		},
		{
			name: "runtime nil type guard proves falsiness",
			src: `
local value = test
if type(value) == "nil" then
	if value then
		return value
	end
end
`,
			want: "always false",
		},
		{
			name: "literal guard makes opposite literal check impossible",
			src: `
local item = { kind = "ready" }
if item.kind == "ready" then
	if item.kind ~= "ready" then
		return item
	end
end
`,
			want: "always false",
		},
		{
			name: "repeat body preserves incoming truthy guard",
			src: `
local value = test
if value then
	repeat
		if value then
			return value
		end
	until test
end
`,
			want: "always true",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			diags := ProduceWithConfig(runDiagnosticsResult(t, tt.src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
				CodeRedundantCondition: diagnostic.Enable(),
			}}})
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %#v, want one redundant-condition warning", diags)
			}
			if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, tt.want) {
				t.Fatalf("diagnostic = %#v, want %s redundant-condition warning", d, tt.want)
			}
		})
	}
}

func TestRedundantConditionWarningSkipsNestedImpossibleEdges(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local item = { kind = "ready" }
if item.kind == "ready" then
	if item.kind == "other" then
		if item.kind == "other" then
			return item
		end
	end
end
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want only the first impossible branch warning", diags)
	}
	if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always false") {
		t.Fatalf("diagnostic = %#v, want first impossible branch to be reported as always false", d)
	}
}

func TestUnreachableBranchBodySuppressesOtherDiagnostics(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
type Ready = { kind: "ready", ok: string }
local item: Ready = { kind = "ready", ok = "yes" }
if item.kind == "ready" then
	if item.kind == "other" then
		local wrong: number = "bad"
		local maybe: string? = nil
		local joined = "prefix" .. maybe
		local call_target: number = 1
		call_target()
		local missing = item.nope
	end
end
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want only the impossible-branch warning", diags)
	}
	if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always false") {
		t.Fatalf("diagnostic = %#v, want redundant-condition warning only", d)
	}
}

func TestSolvedUnreachableBranchSuppressesAssignmentDiagnostic(t *testing.T) {
	diags := Produce(runDiagnosticsResult(t, `
local x: string? = nil
if x ~= nil then
	local s: string = x
end
`))
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for assignment in solved-unreachable branch", diags)
	}
}

func TestUnreachableBranchBodySuppressesUnusedLocalLint(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local item = { kind = "ready" }
if item.kind == "ready" then
	if item.kind == "other" then
		local unreachable_unused = 1
	end
end
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
		CodeUnusedLocal:        diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want only the impossible-branch warning", diags)
	}
	if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always false") {
		t.Fatalf("diagnostic = %#v, want redundant-condition warning only", d)
	}
}

func TestUnreachableFunctionDefinitionDoesNotSatisfyLaterDirectCall(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local f: number = 1
local item = { kind = "ready" }
if item.kind == "ready" then
	if item.kind == "other" then
		f = function(value: string): () end
	end
end
f("ok")
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}})
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %#v, want impossible-branch warning and later not-callable error", diags)
	}
	if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always false") {
		t.Fatalf("first diagnostic = %#v, want redundant-condition warning", d)
	}
	if d := diags[1]; d.Code != CodeDirectCallNotCallable || !strings.Contains(d.Message, "f is number, not callable") {
		t.Fatalf("second diagnostic = %#v, want not-callable error for later call", d)
	}
}

func TestUnreachableFreezeCallDoesNotSupportLaterMutationWarning(t *testing.T) {
	result := runDiagnosticsResultFull(t, `
local cfg = { name = "prod" }
local item = { kind = "ready" }
if item.kind == "ready" then
	if item.kind == "other" then
		table.freeze(cfg)
	end
end
cfg.name = "staging"
`, []string{"table"}, signaturelookup.Source{IncludeStdlib: true})
	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeFrozenTableMutation: diagnostic.Enable(),
		CodeRedundantCondition:  diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want only the impossible-branch warning", diags)
	}
	if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always false") {
		t.Fatalf("diagnostic = %#v, want redundant-condition warning only", d)
	}
}

func TestRedundantConditionWarningUsesSignatureCallInvalidation(t *testing.T) {
	policy := Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}}
	t.Run("pure signature preserves member guard", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local box = { value = test }
if box.value then
	pure(box)
	if box.value then
		return box
	end
end
`, []string{"test", "pure"}, redundantConditionSignatureSource())
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 1 {
			t.Fatalf("diagnostics = %#v, want one redundant-condition warning", diags)
		}
		if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always true") {
			t.Fatalf("diagnostic = %#v, want member guard to remain always true across pure call", d)
		}
	})
	t.Run("mutating signature invalidates member guard", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local box = { value = test }
if box.value then
	mutate(box)
	if box.value then
		return box
	end
end
`, []string{"test", "mutate"}, redundantConditionSignatureSource())
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 0 {
			t.Fatalf("diagnostics = %#v, want mutating signature to invalidate member guard", diags)
		}
	})
	t.Run("mutating unrelated argument preserves member guard", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local box = { value = test }
local other = {}
if box.value then
	mutate(other)
	if box.value then
		return box
	end
end
`, []string{"test", "mutate"}, redundantConditionSignatureSource())
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 1 {
			t.Fatalf("diagnostics = %#v, want unrelated mutation to preserve member guard", diags)
		}
		if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always true") {
			t.Fatalf("diagnostic = %#v, want member guard to remain always true", d)
		}
	})
	t.Run("mutating signature preserves container root guard", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local box = { value = test }
if box then
	mutate(box)
	if box then
		return box
	end
end
`, []string{"test", "mutate"}, redundantConditionSignatureSource())
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 1 {
			t.Fatalf("diagnostics = %#v, want root guard preserved across descendant invalidation", diags)
		}
		if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always true") {
			t.Fatalf("diagnostic = %#v, want root guard to remain always true", d)
		}
	})
	t.Run("operational path invalidation preserves container root only", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local box = { value = test }
if box then
	if box.value then
		invalidate_children(box)
		if box then
			local root = box
		end
		if box.value then
			local stale = box.value
		end
	end
end
`, []string{"test", "invalidate_children"}, redundantConditionSignatureSource())
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 1 {
			t.Fatalf("diagnostics = %#v, want only container-root redundant-condition warning", diags)
		}
		if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always true") ||
			!strings.Contains(d.Explanation.String(), "box is truthy") {
			t.Fatalf("diagnostic = %#v, want root guard proof after descendant invalidation", d)
		}
	})
	t.Run("open signature is not treated as no-effect proof", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local value = test
if value then
	unknown(value)
	if value then
		return value
	end
end
`, []string{"test", "unknown"}, redundantConditionSignatureSource())
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 0 {
			t.Fatalf("diagnostics = %#v, want open-effect signature to clear guard proof", diags)
		}
	})
	t.Run("stdlib type signature preserves guard", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local value = test
if value then
	local tag = type(value)
	if value then
		return tag
	end
end
`, []string{"test", "type"}, signaturelookup.Source{IncludeStdlib: true})
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 1 {
			t.Fatalf("diagnostics = %#v, want stdlib type call to preserve guard proof", diags)
		}
		if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always true") {
			t.Fatalf("diagnostic = %#v, want value guard to remain always true", d)
		}
	})
	t.Run("stdlib assert signature preserves guard", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local value = test
if value then
	assert(value)
	if value then
		return value
	end
end
`, []string{"test", "assert"}, signaturelookup.Source{IncludeStdlib: true})
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 1 {
			t.Fatalf("diagnostics = %#v, want stdlib assert call to preserve guard proof", diags)
		}
		if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always true") {
			t.Fatalf("diagnostic = %#v, want value guard to remain always true", d)
		}
	})
	t.Run("shadowed stdlib name is not trusted by spelling", func(t *testing.T) {
		result := runDiagnosticsResultFull(t, `
local value = test
local function type(x)
	value = false
	return "boolean"
end
if value then
	type(value)
	if value then
		return value
	end
end
`, []string{"test"}, signaturelookup.Source{IncludeStdlib: true})
		diags := ProduceWithConfig(result, policy)
		if len(diags) != 0 {
			t.Fatalf("diagnostics = %#v, want local shadow to block stdlib purity proof", diags)
		}
	})
}

func redundantConditionSignatureSource() signaturelookup.Source {
	m := manifest.New("redundant")
	m.DefineFunctionSignature("pure", signature.Function{
		Type: typ.Func().
			Param("box", typ.Any).
			Build(),
		Effect: effect.Empty,
	})
	m.DefineFunctionSignature("mutate", signature.Function{
		Type: typ.Func().
			Param("box", typ.Any).
			Build(),
		Effect: effect.Row{Labels: []effect.Label{
			mutation.Mutate{
				Target:    effect.ParamRef{Index: 0},
				Transform: mutation.Unchanged{},
			},
		}},
	})
	m.DefineFunctionSignature("invalidate_children", signature.Function{
		Type: typ.Func().
			Param("box", typ.Any).
			Build(),
		OperationalEffects: &signature.OperationalEffects{
			PathInvalidations: []signature.PathInvalidation{{
				Path: path.NewPlaceholder(0),
			}},
		},
	})
	m.DefineFunctionSignature("unknown", signature.Function{
		Type: typ.Func().
			Param("value", typ.Any).
			Build(),
		Effect: effect.Unknown,
	})
	return signaturelookup.Source{Manifests: []*manifest.Manifest{m}}
}

func TestRedundantConditionWarningRespectsInvalidationAndWeakProofs(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "root assignment invalidates truthy guard",
			src: `
local value = test
if value then
	value = false
	if value then
		return value
	end
end
`,
		},
		{
			name: "unknown call invalidates captured local guard",
			src: `
local value = test
local function mutate()
	value = false
end
if value then
	mutate()
	if value then
		return value
	end
end
`,
		},
		{
			name: "unknown call invalidates table member guard",
			src: `
local function mutate(box)
	box.value = false
end
local box = { value = test }
if box.value then
	mutate(box)
	if box.value then
		return box
	end
end
`,
		},
		{
			name: "method call invalidates receiver member guard",
			src: `
local box = { value = test }
function box:mutate()
	self.value = false
end
if box.value then
	box:mutate()
	if box.value then
		return box
	end
end
`,
		},
		{
			name: "method call with captured receiver reassignment invalidates receiver root guard",
			src: `
local value = {}
function value:clear()
	value = false
end
if value then
	value:clear()
	if value then
		return value
	end
end
`,
		},
		{
			name: "call return assignment invalidates root guard",
			src: `
local value = test
local function make()
	return false
end
if value then
	value = make()
	if value then
		return value
	end
end
`,
		},
		{
			name: "call on one join path prevents all-path guard proof",
			src: `
local value = test
local function mutate()
	value = false
end
if test then
	mutate()
end
if value then
	return value
end
`,
		},
		{
			name: "loop call prevents stale entry guard proof",
			src: `
local value = test
local function mutate()
	value = false
end
if value then
	while test do
		mutate()
	end
	if value then
		return value
	end
end
`,
		},
		{
			name: "not-nil guard is not a truthy proof",
			src: `
local value = test
if value ~= nil then
	if value then
		return value
	end
end
`,
		},
		{
			name: "negative literal proof does not imply a different literal",
			src: `
local item = { kind = test }
if item.kind ~= "ready" then
	if item.kind == "other" then
		return item
	end
end
`,
		},
		{
			name: "dynamic index write invalidates descendant guard",
			src: `
local box = { value = test }
local key = test
if box.value then
	box[key] = false
	if box.value then
		return box
	end
end
`,
		},
		{
			name: "alias static member write invalidates possibly aliased guard",
			src: `
local box = { value = test }
local alias = box
if box.value then
	alias.value = false
	if box.value then
		return box
	end
end
`,
		},
		{
			name: "post-branch join drops guard known on only one edge",
			src: `
local value = test
if value then
end
if value then
return value
end
`,
		},
		{
			name: "diamond join drops nested guard known on only some paths",
			src: `
local value = test
local gate = test
local other = test
if gate then
	if value then
	end
else
	if other then
		if value then
		end
	end
end
if value then
	return value
end
`,
		},
		{
			name: "loop backedge assignment prevents all-path guard proof",
			src: `
local value = test
if value then
	while test do
		value = false
	end
	if value then
		return value
	end
end
`,
		},
		{
			name: "boolean runtime type guard is not a truthy proof",
			src: `
local value = test
if type(value) == "boolean" then
	if value then
		return value
	end
end
`,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			diags := ProduceWithConfig(runDiagnosticsResult(t, tt.src), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
				CodeRedundantCondition: diagnostic.Enable(),
			}}})
			if len(diags) != 0 {
				t.Fatalf("diagnostics = %#v, want no redundant-condition warning", diags)
			}
		})
	}
}

func TestRedundantConditionWarningPreservesSiblingGuardAfterStaticWrite(t *testing.T) {
	result := runDiagnosticsResult(t, `
local box = { value = test, other = test }
if box.other then
	box.value = false
	if box.other then
		return box
	end
end
`)
	diags := ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeRedundantCondition: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want same-root sibling guard to survive static member write", diags)
	}
	if d := diags[0]; d.Code != CodeRedundantCondition || !strings.Contains(d.Message, "always true") {
		t.Fatalf("diagnostic = %#v, want sibling guard to remain always true", d)
	}
	if evidence := diags[0].Explanation.Evidence(); len(evidence) != 3 ||
		!strings.Contains(evidence[2].Message, "box.other is unchanged between the prior guard and this check") {
		t.Fatalf("evidence = %#v, want invalidation evidence for box.other", evidence)
	}
}
