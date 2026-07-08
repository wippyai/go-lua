package lua

import (
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestFixtureDiagnosticsRequireEvidenceRenderAndLabels(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	for _, suite := range suites {
		if suite.Suite.Check == nil {
			continue
		}
		for i, exp := range suite.Suite.Check.Diagnostics {
			if len(exp.Evidence) == 0 {
				t.Errorf("%s diagnostic %d must assert exact evidence", suite.Name, i)
			}
			if len(exp.Labels) == 0 {
				t.Errorf("%s diagnostic %d must assert exact labels", suite.Name, i)
			}
			if len(exp.RenderOrderedContains) == 0 {
				t.Errorf("%s diagnostic %d must assert ordered rendered output", suite.Name, i)
			}
			if len(exp.RenderNotContains) == 0 {
				t.Errorf("%s diagnostic %d must assert forbidden rendered output", suite.Name, i)
			}
			if len(exp.HelpContains) == 0 {
				t.Errorf("%s diagnostic %d must assert help text", suite.Name, i)
			}
		}
	}
}

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
	renderOptions := diag.RenderOptions{}
	if !matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("structured diagnostic expectation did not match")
	}

	exp.Column = 2
	if matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("expectation matched wrong column")
	}
}

func TestDiagnosticExpectationCanMatchRenderedOutput(t *testing.T) {
	d := diag.Diagnostic{
		Position: diag.Position{File: "test.lua", Line: 1, Column: 7},
		Span:     diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 15},
		Code:     diag.Code("type.assignment"),
		Severity: diag.SeverityError,
		Message:  "cannot assign nil to string",
		Explanation: diag.NewExplanation(diag.Evidence{
			Kind:    diag.EvidenceMissingProof,
			Trust:   diag.TrustUnknown,
			Span:    diag.Span{StartLine: 1, StartCol: 7},
			Message: "no proof on this path shows this value is string",
		}),
		Labels: []diag.Label{{Span: diag.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 15}, Message: "assigned value"}},
	}
	exp := fixtureDiagnosticExpectation{
		File:             "main.lua",
		Line:             1,
		Column:           7,
		Severity:         "error",
		Code:             "type.assignment",
		MessageContains:  []string{"cannot assign"},
		MinEvidence:      1,
		EvidenceContains: []string{"no proof on this path shows this value is string"},
		RenderContains: []string{
			"error[type.assignment]: cannot assign nil to string",
			"--> main.lua:1:7",
			"local x = nil",
			"assigned value",
			"because:",
			"missing proof",
		},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign nil to string",
			"--> main.lua:1:7",
			"local x = nil",
			"because:",
			"missing proof: no proof on this path shows this value is string",
		},
		RenderNotContains: []string{
			"^~",
		},
		MinLabels:     1,
		LabelContains: []string{"assigned value"},
	}
	renderOptions := diag.RenderOptions{
		Sources:             diag.SourceMap{"main.lua": "local x = nil"},
		DisplayFiles:        map[string]string{"test.lua": "main.lua"},
		ShowSourceLabelRows: true,
	}
	if !matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("structured diagnostic expectation did not match rendered output:\n%s", diag.Render(d, renderOptions))
	}
	exp.RenderContains = []string{"not present"}
	if matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("structured diagnostic expectation matched missing rendered text")
	}
	exp.RenderContains = []string{"cannot assign nil to string"}
	exp.RenderOrderedContains = []string{"because:", "--> main.lua:1:7"}
	if matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("structured diagnostic expectation matched rendered text in the wrong order")
	}
	exp.RenderOrderedContains = []string{"error[type.assignment]:", "because:"}
	exp.RenderNotContains = []string{"cannot assign nil to string"}
	if matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("structured diagnostic expectation matched forbidden rendered text")
	}
}

func TestDiagnosticExpectationCanMatchSpanAnchoredLabels(t *testing.T) {
	d := diag.Diagnostic{
		Position: diag.Position{File: "test.lua", Line: 3, Column: 27},
		Span:     diag.Span{StartLine: 3, StartCol: 27, EndLine: 3, EndCol: 39},
		Code:     diag.Code("type.call.direct.argument_type"),
		Severity: diag.SeverityError,
		Message:  "cannot pass cached.content as argument 1 because it may be nil",
		Explanation: diag.NewExplanation(diag.Evidence{
			Kind:    diag.EvidenceAbstractFact,
			Trust:   diag.TrustProven,
			Message: "cached.content can be string or nil here",
		}),
		Labels: []diag.Label{
			{Span: diag.Span{StartLine: 3, StartCol: 17, EndLine: 3, EndCol: 23}, Message: "argument target"},
			{Span: diag.Span{StartLine: 3, StartCol: 27, EndLine: 3, EndCol: 41}, Message: "argument value"},
		},
		Help: "Guard `cached.content` with a nil check before passing it.",
	}
	exp := fixtureDiagnosticExpectation{
		File:             "main.lua",
		Line:             3,
		Column:           27,
		Severity:         "error",
		Code:             "type.call.direct.argument_type",
		MessageContains:  []string{"cannot pass cached.content"},
		MinEvidence:      1,
		EvidenceContains: []string{"cached.content can be string or nil"},
		RenderContains:   []string{"argument value"},
		HelpContains:     []string{"Guard `cached.content`"},
		MinLabels:        2,
		Labels: []fixtureDiagnosticLabelExpectation{
			{File: "main.lua", Line: 3, Column: 17, Contains: []string{"argument target"}},
			{File: "main.lua", Line: 3, Column: 27, Contains: []string{"argument value"}},
		},
	}
	renderOptions := diag.RenderOptions{
		Sources:             diag.SourceMap{"main.lua": "local cached = store:get(id)\nif cached then\n    now:sub(cached.content)\nend"},
		DisplayFiles:        map[string]string{"test.lua": "main.lua"},
		ShowSourceLabelRows: true,
	}
	if !matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("structured diagnostic expectation did not match span-anchored labels")
	}

	exp.Labels[1].Column = 28
	if matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("span-anchored label expectation matched wrong column")
	}
}

func TestDiagnosticExpectationCanMatchOrderedEvidenceChain(t *testing.T) {
	d := diag.Diagnostic{
		Position: diag.Position{File: "test.lua", Line: 4, Column: 29},
		Span:     diag.Span{StartLine: 4, StartCol: 29, EndLine: 4, EndCol: 43},
		Code:     diag.Code("type.assignment"),
		Severity: diag.SeverityError,
		Message:  "cannot assign cached.content because it may be nil",
		Explanation: diag.NewExplanation(
			diag.Evidence{
				Kind:    diag.EvidenceAbstractFact,
				Trust:   diag.TrustProven,
				Span:    diag.Span{StartLine: 4, StartCol: 29, EndLine: 4, EndCol: 43},
				Message: "cached.content can be string or nil here",
			},
			diag.Evidence{
				Kind:    diag.EvidenceUserAssertion,
				Trust:   diag.TrustClaimed,
				File:    "protocol",
				Span:    diag.Span{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 19},
				Message: "content is declared as string",
			},
			diag.Evidence{
				Kind:    diag.EvidenceMissingProof,
				Trust:   diag.TrustUnknown,
				Reason:  diag.EvidenceReasonBoundaryValidationMissing,
				Span:    diag.Span{StartLine: 4, StartCol: 29, EndLine: 4, EndCol: 43},
				Message: "no guard on this path proves cached.content is non-nil",
			},
		),
		Labels: []diag.Label{{Span: diag.Span{StartLine: 4, StartCol: 29, EndLine: 4, EndCol: 43}, Message: "assigned value"}},
		Help:   "Guard `cached.content` with a nil check.",
	}
	exp := fixtureDiagnosticExpectation{
		File:            "main.lua",
		Line:            4,
		Column:          29,
		Severity:        "error",
		Code:            "type.assignment",
		MessageContains: []string{"cannot assign cached.content"},
		MinEvidence:     3,
		Evidence: []fixtureDiagnosticEvidenceExpectation{
			{File: "main.lua", Line: 4, Column: 29, Kind: "abstract fact", Trust: "proven", Contains: []string{"cached.content can be string or nil"}},
			{File: "protocol.lua", Line: 2, Column: 3, Kind: "user assertion", Trust: "claimed", Contains: []string{"content is declared as string"}},
			{File: "main.lua", Line: 4, Column: 29, Kind: "missing proof", Trust: "unknown", Reason: "boundary validation missing", Contains: []string{"no guard on this path proves"}},
		},
		RenderContains: []string{"cached.content"},
		HelpContains:   []string{"Guard `cached.content`"},
		MinLabels:      1,
		LabelContains:  []string{"assigned value"},
	}
	renderOptions := diag.RenderOptions{DisplayFiles: map[string]string{"test.lua": "main.lua", "protocol": "protocol.lua"}}
	if !matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("structured diagnostic expectation did not match ordered evidence chain")
	}

	exp.Evidence[0], exp.Evidence[1] = exp.Evidence[1], exp.Evidence[0]
	if matchesDiagnosticExpectation(exp, d, "main.lua", renderOptions) {
		t.Fatalf("ordered evidence expectation matched out-of-order evidence")
	}
}

func TestDiagnosticFileMatchingUsesExactFixtureAliases(t *testing.T) {
	d := diag.Diagnostic{Position: diag.Position{File: "protocol", Line: 1}}
	if !matchesDiagnosticFile("protocol.lua", d, "main.lua") {
		t.Fatalf("protocol.lua should match module diagnostic protocol")
	}
	if matchesDiagnosticFile("myprotocol.lua", d, "main.lua") {
		t.Fatalf("myprotocol.lua should not suffix-match protocol")
	}
	entry := diag.Diagnostic{Position: diag.Position{File: "test.lua", Line: 1}}
	if !matchesDiagnosticFile("main.lua", entry, "main.lua") {
		t.Fatalf("entry fixture file should match internal test.lua")
	}
	if matchesDiagnosticFile("protocol.lua", entry, "main.lua") {
		t.Fatalf("non-entry fixture file should not match internal test.lua")
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
	if matchesDiagnosticExpectation(exp, d, "main.lua", diag.RenderOptions{}) {
		t.Fatalf("expectation matched malformed severity")
	}

	exp.Severity = "error"
	exp.MessageContains = []string{""}
	if matchesDiagnosticExpectation(exp, d, "main.lua", diag.RenderOptions{}) {
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
		RenderContains:   []string{"assigned value"},
		RenderOrderedContains: []string{
			"cannot assign",
			"because:",
			"fix",
		},
		RenderNotContains: []string{"missing proof:"},
		HelpContains:      []string{"fix"},
		MinLabels:         1,
		LabelContains:     []string{"assigned value"},
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
			name: "render not asserted",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.RenderContains = nil
			},
			want: "render_contains must contain at least one assertion",
		},
		{
			name: "empty ordered render assertion",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.RenderOrderedContains = []string{"cannot assign", " "}
			},
			want: "render_ordered_contains contains an empty assertion",
		},
		{
			name: "empty forbidden render assertion",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.RenderNotContains = []string{" "}
			},
			want: "render_not_contains contains an empty assertion",
		},
		{
			name: "help not asserted",
			edit: func(exp *fixtureDiagnosticExpectation) {
				exp.HelpContains = nil
			},
			want: "help_contains must contain at least one assertion",
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

func TestDiagnosticExpectationValidationRejectsMissingLabelAssertions(t *testing.T) {
	exp := fixtureDiagnosticExpectation{
		File:             "main.lua",
		Line:             1,
		Severity:         "warning",
		Code:             "lint.dead.assignment",
		MessageContains:  []string{"overwritten"},
		MinEvidence:      2,
		EvidenceContains: []string{"later write", "every path"},
		RenderContains:   []string{"overwritten"},
		HelpContains:     []string{"Remove"},
	}
	if err := validateDiagnosticExpectation(exp); err == nil || !strings.Contains(err.Error(), "label_contains") {
		t.Fatalf("validation error = %v, want missing label assertion", err)
	}

	exp.Labels = []fixtureDiagnosticLabelExpectation{{Line: 1, Column: 7, Contains: []string{"dead assignment"}}}
	if err := validateDiagnosticExpectation(exp); err != nil {
		t.Fatalf("span-anchored label expectation should satisfy label assertions: %v", err)
	}
	exp.Labels[0].Contains = []string{" "}
	if err := validateDiagnosticExpectation(exp); err == nil || !strings.Contains(err.Error(), "labels[0]: contains contains an empty assertion") {
		t.Fatalf("validation error = %v, want malformed label assertion", err)
	}
}

func TestDiagnosticExpectationValidationAcceptsStructuredEvidence(t *testing.T) {
	exp := fixtureDiagnosticExpectation{
		File:            "main.lua",
		Line:            1,
		Severity:        "error",
		Code:            "type.assignment",
		MessageContains: []string{"cannot assign"},
		Evidence: []fixtureDiagnosticEvidenceExpectation{
			{Line: 1, Column: 7, Kind: "missing proof", Trust: "unknown", Contains: []string{"no proof"}},
		},
		RenderContains: []string{"cannot assign"},
		HelpContains:   []string{"fix"},
		MinLabels:      1,
		LabelContains:  []string{"assigned value"},
	}
	if err := validateDiagnosticExpectation(exp); err != nil {
		t.Fatalf("structured evidence expectation should satisfy evidence assertions: %v", err)
	}

	exp.Evidence[0].Kind = "vibes"
	if err := validateDiagnosticExpectation(exp); err == nil || !strings.Contains(err.Error(), `evidence[0]: unknown kind "vibes"`) {
		t.Fatalf("validation error = %v, want malformed evidence kind", err)
	}
}

func TestDiagnosticExpectationValidationAllowsExplicitEmptyEvidenceOptOut(t *testing.T) {
	exp := fixtureDiagnosticExpectation{
		File:               "main.lua",
		Line:               1,
		Severity:           "error",
		Code:               "parse",
		MessageContains:    []string{"syntax"},
		RenderContains:     []string{"syntax"},
		HelpContains:       []string{"fix"},
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
			Help:        "fix the assignment",
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
		RenderContains:   []string{"cannot assign number to string"},
		HelpContains:     []string{"fix the assignment"},
		MinLabels:        1,
		LabelContains:    []string{"assigned value"},
	}}

	renderOptions := diag.RenderOptions{}
	missing, unexpected := matchDiagnosticExpectations(expectations, diags, "main.lua", true, renderOptions)
	if len(missing) != 0 {
		t.Fatalf("missing = %#v, want none", missing)
	}
	if len(unexpected) != 1 {
		t.Fatalf("unexpected = %#v, want one unmatched diagnostic", unexpected)
	}

	_, unexpected = matchDiagnosticExpectations(expectations, diags, "main.lua", false, renderOptions)
	if len(unexpected) != 0 {
		t.Fatalf("unexpected = %#v, want none when complete-list mode is disabled", unexpected)
	}
}

func TestFixtureDiagnosticRenderOptionsAliasEntryAndModules(t *testing.T) {
	opts := fixtureDiagnosticRenderOptions(map[string]string{
		"protocol.lua": "type Receipt = {\n  content: string?,\n}",
		"main.lua":     "local bad_content: string = cached.content",
	}, "main.lua")

	entryRendered := diag.Render(diag.Diagnostic{
		Position: diag.Position{File: "test.lua", Line: 1, Column: 29},
		Span:     diag.Span{StartLine: 1, StartCol: 29, EndLine: 1, EndCol: 43},
		Code:     diag.Code("type.assignment"),
		Severity: diag.SeverityError,
		Message:  "cannot assign string? to string",
	}, opts)
	if !strings.Contains(entryRendered, "--> main.lua:1:29") ||
		!strings.Contains(entryRendered, "local bad_content: string = cached.content") ||
		strings.Contains(entryRendered, "--> test.lua") {
		t.Fatalf("entry render did not use fixture alias:\n%s", entryRendered)
	}

	moduleRendered := diag.Render(diag.Diagnostic{
		Position: diag.Position{File: "protocol", Line: 2, Column: 3},
		Span:     diag.Span{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 19},
		Code:     diag.Code("type.assignment"),
		Severity: diag.SeverityError,
		Message:  "content is optional",
	}, opts)
	if !strings.Contains(moduleRendered, "--> protocol.lua:2:3") ||
		!strings.Contains(moduleRendered, "content: string?,") ||
		strings.Contains(moduleRendered, "--> protocol:") {
		t.Fatalf("module render did not use fixture alias:\n%s", moduleRendered)
	}
}

func TestFixtureDiagnosticRenderOptionsCanEnableWitnessTrace(t *testing.T) {
	opts := fixtureDiagnosticRenderOptions(map[string]string{
		"main.lua": "local x: number = h",
	}, "main.lua", fixtureDiagnosticRenderConfig{WitnessTrace: true})

	if !opts.WitnessTrace {
		t.Fatal("fixture render options did not enable witness trace")
	}
	if !opts.ShowSourceLabelRows {
		t.Fatal("fixture render options should keep source label rows enabled")
	}
}

func TestDiagnosticRenderPolicyRejectsNoisySourceFrames(t *testing.T) {
	tests := []struct {
		name     string
		rendered string
		want     string
	}{
		{
			name: "span underline",
			rendered: "error[x]: bad\n" +
				" --> main.lua:1:7\n" +
				"  |\n" +
				"1 | local x = nil\n" +
				"  |       ^~~~~ assigned value",
			want: "exact carets",
		},
		{
			name: "label row split from caret",
			rendered: "error[x]: bad\n" +
				" --> main.lua:1:7\n" +
				"  |\n" +
				"1 | local x = nil\n" +
				"  |       assigned value\n" +
				"  |       ^",
			want: "directional arrow",
		},
		{
			name: "label row plus caret layer",
			rendered: "error[x]: bad\n" +
				" --> main.lua:1:7\n" +
				"  |\n" +
				"  |       assigned value\n" +
				"1 | local x = nil\n" +
				"  |       ^",
			want: "directional arrow",
		},
		{
			name: "unlabeled and labeled carets in one frame",
			rendered: "error[x]: bad\n" +
				" --> main.lua:1:17\n" +
				"  |\n" +
				"1 | local elapsed = now:sub(t)\n" +
				"  |                 ^\n" +
				"  |                         ^ argument value",
			want: "must not mix",
		},
		{
			name: "multiple labeled caret rows",
			rendered: "error[x]: bad\n" +
				" --> main.lua:1:17\n" +
				"  |\n" +
				"1 | local name: string = raw.name\n" +
				"  |       ^ declared type\n" +
				"  |                     ^ assigned value",
			want: "directional arrows",
		},
		{
			name: "crowded labels on caret row",
			rendered: "error[x]: bad\n" +
				" --> main.lua:1:27\n" +
				"  |\n" +
				"1 | local got_count: number = load_name()\n" +
				"  |    declared type ^        ^ call result",
			want: "directional arrows",
		},
		{
			name: "single label on caret row",
			rendered: "error[x]: bad\n" +
				" --> main.lua:1:7\n" +
				"  |\n" +
				"1 | local x = nil\n" +
				"  |       ^ assigned value",
			want: "directional arrows",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			violations := renderedDiagnosticFramePolicyViolations(tc.rendered)
			if tc.want == "" {
				if len(violations) != 0 {
					t.Fatalf("violations = %v, want none", violations)
				}
				return
			}
			if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), tc.want) {
				t.Fatalf("violations = %v, want %q", violations, tc.want)
			}
		})
	}
}

func TestDiagnosticRenderPolicyAcceptsLabelAboveSingleSourceFrame(t *testing.T) {
	rendered := "error[x]: bad\n" +
		" --> main.lua:1:7\n" +
		"  |\n" +
		"1 | local x = nil\n" +
		"  |       ↑ assigned value\n" +
		"\n" +
		"because:\n" +
		"  1. proven: x is nil"
	if violations := renderedDiagnosticFramePolicyViolations(rendered); len(violations) > 0 {
		t.Fatalf("violations = %v, want none", violations)
	}
}

func TestDiagnosticRenderPolicyAcceptsLabelsAboveMultiLabelSourceFrame(t *testing.T) {
	rendered := "error[x]: bad\n" +
		" --> main.lua:1:21\n" +
		"  |\n" +
		"  |       ↓ declared type\n" +
		"1 | local name: string = raw.name\n" +
		"  |                     ↑ assigned value\n" +
		"\n" +
		"because:\n" +
		"  1. proven: raw.name has type string?"
	if violations := renderedDiagnosticFramePolicyViolations(rendered); len(violations) > 0 {
		t.Fatalf("violations = %v, want none", violations)
	}
}

func TestFixtureDiagnosticRuleOptionsEnableOptInDiagnostics(t *testing.T) {
	enabled := true
	opts, err := fixtureDiagnosticRuleOptions(&fixtureCheck{DiagnosticRules: []fixtureDiagnosticRule{
		{Code: diagnostics.CodeUnusedLocal.String(), Enabled: &enabled, Severity: "hint"},
	}})
	if err != nil {
		t.Fatalf("fixtureDiagnosticRuleOptions returned error: %v", err)
	}
	result := testutil.Check(`local unused = 1`, opts...)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want unused-local diagnostic", result.Diagnostics)
	}
	d := result.Diagnostics[0]
	if d.Code != diagnostics.CodeUnusedLocal || d.Severity != diag.SeverityHint {
		t.Fatalf("diagnostic = %#v, want unused-local hint", d)
	}
}

func TestFixtureDiagnosticRuleOptionsRejectMalformedRules(t *testing.T) {
	enabled := true
	for _, tc := range []struct {
		name string
		rule fixtureDiagnosticRule
		want string
	}{
		{
			name: "missing code",
			rule: fixtureDiagnosticRule{Enabled: &enabled},
			want: "code is required",
		},
		{
			name: "missing action",
			rule: fixtureDiagnosticRule{Code: diagnostics.CodeUnusedLocal.String()},
			want: "must set enabled or severity",
		},
		{
			name: "bad severity",
			rule: fixtureDiagnosticRule{Code: diagnostics.CodeUnusedLocal.String(), Severity: "notice"},
			want: "unknown severity",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fixtureDiagnosticRuleOptions(&fixtureCheck{DiagnosticRules: []fixtureDiagnosticRule{tc.rule}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
