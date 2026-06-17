package lua

import (
	"strings"
	"testing"

	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiagnosticExpectationMatchesEvidenceLabelsAndPosition(t *testing.T) {
	d := diag.Diagnostic{
		Position: diag.Position{File: "test.lua", Line: 5, Column: 1},
		Code:     diag.Code("type.assignment"),
		Severity: diag.SeverityError,
		Message:  "cannot assign number to string",
		Explanation: diag.NewExplanation(diag.Evidence{
			Kind:    diag.EvidenceAbstractFact,
			Trust:   diag.TrustProven,
			Message: "source expression is number",
		}),
		Labels: []diag.Label{{Message: "assigned value"}},
	}

	exp := fixtureDiagnosticExpectation{
		File:             "main.lua",
		Line:             5,
		Column:           1,
		Severity:         "error",
		Code:             "type.assignment",
		MessageContains:  []string{"cannot assign", "string"},
		MinEvidence:      1,
		EvidenceContains: []string{"source expression"},
		MinLabels:        1,
		LabelContains:    []string{"assigned value"},
	}
	if !matchesDiagnosticExpectation(exp, d, "main.lua") {
		t.Fatalf("structured diagnostic expectation did not match")
	}

	exp.Column = 2
	if matchesDiagnosticExpectation(exp, d, "main.lua") {
		t.Fatalf("expectation matched wrong column")
	}
}

func TestDiagnosticExpectationRejectsMalformedAssertions(t *testing.T) {
	d := diag.Diagnostic{
		Position:    diag.Position{File: "test.lua", Line: 1, Column: 1},
		Code:        diag.Code("type.assignment"),
		Severity:    diag.SeverityError,
		Message:     "cannot assign number to string",
		Explanation: diag.NewExplanation(diag.Evidence{Message: "source expression is number"}),
		Labels:      []diag.Label{{Message: "assigned value"}},
	}

	exp := fixtureDiagnosticExpectation{
		File:            "main.lua",
		Line:            1,
		Column:          1,
		Severity:        "warnig",
		Code:            "type.assignment",
		MessageContains: []string{"cannot assign"},
	}
	if matchesDiagnosticExpectation(exp, d, "main.lua") {
		t.Fatalf("expectation matched malformed severity")
	}

	exp.Severity = "error"
	exp.MessageContains = []string{""}
	if matchesDiagnosticExpectation(exp, d, "main.lua") {
		t.Fatalf("expectation matched empty message assertion")
	}
}

func TestDiagnosticExpectationValidationRejectsWeakStructuredSpecs(t *testing.T) {
	valid := fixtureDiagnosticExpectation{
		File:             "main.lua",
		Line:             1,
		Severity:         "error",
		Code:             "type.assignment",
		MessageContains:  []string{"cannot assign"},
		MinEvidence:      2,
		EvidenceContains: []string{"source expression", "declared type"},
		MinLabels:        1,
		LabelContains:    []string{"assigned value"},
	}
	if err := validateDiagnosticExpectation(valid); err != nil {
		t.Fatalf("valid expectation rejected: %v", err)
	}

	for _, tc := range []struct {
		name string
		edit func(*fixtureDiagnosticExpectation)
		want string
	}{
		{
			name: "missing code",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.Code = ""
			},
			want: "code is required",
		},
		{
			name: "empty message assertion",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.MessageContains = []string{"cannot assign", " "}
			},
			want: "message_contains contains an empty assertion",
		},
		{
			name: "evidence not asserted",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.MinEvidence = 0
				exp.EvidenceContains = nil
			},
			want: "evidence_contains must contain at least one assertion",
		},
		{
			name: "labels not asserted",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.MinLabels = 0
				exp.LabelContains = nil
			},
			want: "label_contains must contain at least one assertion",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exp := valid
			tc.edit(&exp)
			err := validateDiagnosticExpectation(exp)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validation error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDiagnosticExpectationValidationAllowsExplicitEmptyEvidenceOptOut(t *testing.T) {
	exp := fixtureDiagnosticExpectation{
		File:               "main.lua",
		Line:               1,
		Severity:           "error",
		Code:               "parse",
		MessageContains:    []string{"syntax"},
		MinLabels:          1,
		LabelContains:      []string{"parse error"},
		AllowEmptyEvidence: true,
	}
	if err := validateDiagnosticExpectation(exp); err != nil {
		t.Fatalf("explicit empty-evidence opt-out rejected: %v", err)
	}
}

func TestStructuredDiagnosticsCanRequireCompleteList(t *testing.T) {
	diags := []diag.Diagnostic{
		{
			Position:    diag.Position{File: "test.lua", Line: 1, Column: 1},
			Code:        diag.Code("type.assignment"),
			Severity:    diag.SeverityError,
			Message:     "cannot assign number to string",
			Explanation: diag.NewExplanation(diag.Evidence{Message: "source expression is number"}),
			Labels:      []diag.Label{{Message: "assigned value"}},
		},
		{
			Position:    diag.Position{File: "test.lua", Line: 2, Column: 1},
			Code:        diag.Code("type.call.direct.not_callable"),
			Severity:    diag.SeverityError,
			Message:     "target is number, not callable",
			Explanation: diag.NewExplanation(diag.Evidence{Message: "target is annotated number"}),
		},
	}
	expectations := []fixtureDiagnosticExpectation{{
		File:             "main.lua",
		Line:             1,
		Severity:         "error",
		Code:             "type.assignment",
		MessageContains:  []string{"cannot assign"},
		MinEvidence:      1,
		EvidenceContains: []string{"source expression"},
		MinLabels:        1,
		LabelContains:    []string{"assigned value"},
	}}

	missing, unexpected := matchDiagnosticExpectations(expectations, diags, "main.lua", true)
	if len(missing) != 0 {
		t.Fatalf("missing = %#v, want none", missing)
	}
	if len(unexpected) != 1 {
		t.Fatalf("unexpected = %#v, want one unmatched diagnostic", unexpected)
	}

	_, unexpected = matchDiagnosticExpectations(expectations, diags, "main.lua", false)
	if len(unexpected) != 0 {
		t.Fatalf("unexpected = %#v, want none when complete-list mode is disabled", unexpected)
	}
}
