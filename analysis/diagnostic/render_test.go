package diagnostic

import (
	"strings"
	"testing"
)

func TestRenderStyleOwnsFixedTextAndColors(t *testing.T) {
	style := newRenderStyle(true)
	if style.text.fallbackSeverity != "error" ||
		style.text.evidenceSection != "because" ||
		style.text.labelsSection != "where" ||
		style.text.helpPrefix != "help" ||
		style.text.locationArrow != "-->" ||
		style.text.labelFallbackStart != "  = " ||
		style.text.provenHeading != "proven" ||
		style.text.claimedHeading != "claimed" ||
		style.text.refutedHeading != "refuted" ||
		style.text.factHeading != "fact" ||
		style.text.claimHeading != "claim" ||
		style.text.missingProofHeading != "missing proof" ||
		style.text.precisionBoundaryHeader != "unvalidated value" ||
		style.text.evidenceHeading != "evidence" {
		t.Fatalf("render text policy = %#v", style.text)
	}
	if got := style.colorSeverity(SeverityError, "error"); got != "\x1b[1;31merror\x1b[0m" {
		t.Fatalf("error color = %q", got)
	}
	if got := style.evidenceHeading(Evidence{Kind: EvidenceMissingProof, Trust: TrustUnknown}); got != "\x1b[1;31mmissing proof\x1b[0m" {
		t.Fatalf("missing-proof heading = %q", got)
	}
}

func TestRenderEvidenceHeadingDoesNotOverstateKindSpecificEvidence(t *testing.T) {
	style := newRenderStyle(false)
	tests := []struct {
		name string
		item Evidence
		want string
	}{
		{
			name: "missing proof without explicit trust",
			item: Evidence{Kind: EvidenceMissingProof},
			want: "missing proof",
		},
		{
			name: "unvalidated value without explicit trust",
			item: Evidence{Kind: EvidencePrecisionBoundary},
			want: "unvalidated value",
		},
		{
			name: "user assertion without explicit trust",
			item: Evidence{Kind: EvidenceUserAssertion},
			want: "claim",
		},
		{
			name: "abstract fact keeps proven default",
			item: Evidence{Kind: EvidenceAbstractFact},
			want: "proven",
		},
		{
			name: "abstract fact unknown is fact",
			item: Evidence{Kind: EvidenceAbstractFact, Trust: TrustUnknown},
			want: "fact",
		},
		{
			name: "claimed abstract fact",
			item: Evidence{Kind: EvidenceAbstractFact, Trust: TrustClaimed},
			want: "claimed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := style.evidenceHeading(tt.item); got != tt.want {
				t.Fatalf("evidenceHeading = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderDiagnosticWithEvidenceLabelsAndHelp(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 3, Column: 21},
		Span:     Span{StartLine: 3, StartCol: 21, EndLine: 3, EndCol: 29},
		Code:     Code("type.assignment"),
		Message:  "cannot assign any to string",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				Span:    Span{StartLine: 3, StartCol: 21, EndLine: 3, EndCol: 29},
				Message: "source expression is any",
			},
			Evidence{
				Kind:    EvidenceMissingProof,
				Trust:   TrustUnknown,
				Span:    Span{StartLine: 3, StartCol: 21, EndLine: 3, EndCol: 29},
				Message: "no proof on this path shows this value is string",
			},
		),
		Labels: []Label{
			{Span: Span{StartLine: 3, StartCol: 7, EndLine: 3, EndCol: 13}, Message: "declared type"},
			{Span: Span{StartLine: 3, StartCol: 21, EndLine: 3, EndCol: 29}, Message: "assigned value"},
		},
		Help: "validate the value before assigning it to string",
	}

	rendered := Render(d, RenderOptions{
		Sources:             SourceMap{"main.lua": "local raw: any = decode()\nlocal ok = true\nlocal name: string = raw.name"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"error[type.assignment]: cannot assign any to string",
		"--> main.lua:3:21",
		"  |       ↓ declared type",
		"3 | local name: string = raw.name",
		"  |                     ↑ assigned value",
		"because:",
		"1. proven: source expression is any",
		"2. missing proof: no proof on this path shows this value is string",
		"help: validate the value before assigning it to string",
	)
	if strings.Contains(rendered, "^~") {
		t.Fatalf("source-frame markers should use exact carets, not span underlines:\n%s", rendered)
	}
	if strings.Contains(rendered, "\nwhere:\n") {
		t.Fatalf("same-line labels should attach to the primary source frame:\n%s", rendered)
	}
	if strings.Count(rendered, "assigned value") != 1 {
		t.Fatalf("primary label should render once, got:\n%s", rendered)
	}
	if strings.Count(rendered, "\n  |") < 2 ||
		strings.Count(rendered, "\n  |       ↓ declared type") != 1 ||
		strings.Count(rendered, "\n  |                     ↑ assigned value") != 1 {
		t.Fatalf("multiple same-line annotations should render as label rows:\n%s", rendered)
	}
	if strings.Contains(rendered, "\n  |       ^             ^") {
		t.Fatalf("rich label mode should not render a second caret layer:\n%s", rendered)
	}
}

func TestRenderDiagnosticLabelRowsUsePlacementMetadataNotMessageText(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 19},
		Span:     Span{StartLine: 1, StartCol: 19, EndLine: 1, EndCol: 22},
		Code:     Code("type.assignment"),
		Message:  "cannot assign raw to string",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 16}, Message: "declared type", Placement: LabelPlacementBelow},
			{Span: Span{StartLine: 1, StartCol: 19, EndLine: 1, EndCol: 22}, Message: "assigned value", Placement: LabelPlacementAbove},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": "local x: string = raw"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered, "↓ assigned value", "↑ declared type")
	if strings.Contains(rendered, "↑ assigned value") || strings.Contains(rendered, "↓ declared type") {
		t.Fatalf("label placement followed message text instead of metadata:\n%s", rendered)
	}
}

func TestRenderDiagnosticOmitsSourceLabelRowsByDefault(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 22},
		Span:     Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 36},
		Code:     Code("type.assignment"),
		Message:  "cannot assign cached.content because it may be nil",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				Span:    Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 36},
				Message: "cached.content can be string or nil here",
			},
			Evidence{
				Kind:    EvidenceUserAssertion,
				Trust:   TrustClaimed,
				Span:    Span{StartLine: 1, StartCol: 14, EndLine: 1, EndCol: 20},
				Message: "target is declared as string",
			},
		),
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 14, EndLine: 1, EndCol: 20}, Message: "declared type"},
			{Span: Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 36}, Message: "assigned value"},
		},
		Help: "guard cached.content before assigning it to a string",
	}

	rendered := Render(d, RenderOptions{Sources: SourceMap{"main.lua": "local target: string = cached.content"}})

	containsAll(t, rendered,
		"error[type.assignment]: cannot assign cached.content because it may be nil",
		"--> main.lua:1:22",
		"1 | local target: string = cached.content",
		"  |              ^       ^",
		"because:",
		"1. proven: cached.content can be string or nil here",
		"2. claimed: target is declared as string",
		"help: guard cached.content before assigning it to a string",
	)
	if strings.Contains(rendered, "  |              declared type") ||
		strings.Contains(rendered, "  |                      assigned value") {
		t.Fatalf("compact source frame should not render label rows:\n%s", rendered)
	}
	if strings.Contains(rendered, "\nwhere:\n") {
		t.Fatalf("compact source frame should still mark labels rendered:\n%s", rendered)
	}
}

func TestRenderDiagnosticWithoutSourceStillShowsEvidence(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 20, Column: 200},
		Code:     Code("type.member_read"),
		Message:  "record has no field missing",
		Severity: SeverityError,
		Explanation: NewExplanation(Evidence{
			Kind:    EvidenceAbstractFact,
			Trust:   TrustProven,
			Span:    Span{StartLine: 20, StartCol: 200},
			Message: "receiver type at read is {present: string}",
		}),
	}

	rendered := Render(d, RenderOptions{})

	containsAll(t, rendered,
		"error[type.member_read]: record has no field missing",
		"--> main.lua:20:200",
		"because:",
		"receiver type at read is {present: string}",
	)
}

func TestRenderDiagnosticWithoutAnyLocationOmitsBogusFrame(t *testing.T) {
	rendered := Render(Diagnostic{
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
	}, RenderOptions{})

	if strings.Contains(rendered, "-->") || strings.Contains(rendered, "<unknown>") {
		t.Fatalf("locationless diagnostic should not render a bogus frame:\n%s", rendered)
	}
	containsAll(t, rendered, "error[type.assignment]: cannot assign nil to string")
}

func TestRenderDiagnosticFileOnlyEvidenceAndLabels(t *testing.T) {
	rendered := Render(Diagnostic{
		Code:     Code("type.assignment"),
		Message:  "cached.content may be nil here",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				File:    "session_store.lua",
				Message: "cache lookup can miss",
			},
		),
		Labels: []Label{
			{File: "protocol.lua", Message: "Receipt.content is optional"},
		},
	}, RenderOptions{})

	containsAll(t, rendered,
		"because:",
		"1. proven: cache lookup can miss",
		"--> session_store.lua",
		"where:",
		"--> protocol.lua",
		"= Receipt.content is optional",
	)
}

func TestRenderDiagnosticFallbackEvidenceLanguageIsUserReadable(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
		Explanation: NewExplanation(Evidence{
			Kind:  EvidenceMissingProof,
			Trust: TrustUnknown,
			Span:  Span{StartLine: 1, StartCol: 7},
		}),
	}

	rendered := Render(d, RenderOptions{Sources: SourceMap{"main.lua": "local x = nil"}})
	containsAll(t, rendered, "1. missing proof: required proof was not found")
	if strings.Contains(rendered, "evidence is unknown") {
		t.Fatalf("rendered diagnostic leaked internal fallback language:\n%s", rendered)
	}
}

func TestRenderDiagnosticDoesNotOverstateUnknownTrust(t *testing.T) {
	rendered := Render(Diagnostic{
		Code:     Code("type.assignment"),
		Message:  "cannot prove assignment",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{Kind: EvidenceAbstractFact, Trust: TrustUnknown, Message: "source type came from an unvalidated value"},
			Evidence{Kind: EvidenceAbstractFact, Trust: TrustClaimed, Message: "caller claimed a string"},
		),
	}, RenderOptions{})

	containsAll(t, rendered,
		"1. fact: source type came from an unvalidated value",
		"2. claimed: caller claimed a string",
	)
	if strings.Contains(rendered, "proven: source type came from an unvalidated value") {
		t.Fatalf("unknown-trust abstract fact should not render as proven:\n%s", rendered)
	}
}

func TestRenderDiagnosticDeduplicatesExactEvidenceOnly(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 17},
		Span:     Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 20},
		Code:     Code("type.assignment"),
		Message:  "cannot assign value to string",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceMissingProof,
				Trust:   TrustUnknown,
				Span:    Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 20},
				Message: "no proof on this path shows value is string",
			},
			Evidence{
				Kind:    EvidenceMissingProof,
				Trust:   TrustUnknown,
				Span:    Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 20},
				Message: "no proof on this path shows value is string",
			},
			Evidence{
				Kind:    EvidenceMissingProof,
				Trust:   TrustUnknown,
				Span:    Span{StartLine: 2, StartCol: 12, EndLine: 2, EndCol: 17},
				Message: "no proof on this path shows value is string",
			},
		),
		Labels: []Label{{Span: Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 20}, Message: "assigned value"}},
	}, RenderOptions{Sources: SourceMap{"main.lua": "local name: string = raw\nlocal raw = load()"}})

	containsAll(t, rendered,
		"1. missing proof: no proof on this path shows value is string",
		"2. missing proof: no proof on this path shows value is string",
		"--> main.lua:2:12",
	)
	if strings.Contains(rendered, "3. missing proof") {
		t.Fatalf("exact duplicate evidence should be suppressed:\n%s", rendered)
	}
	if got := strings.Count(rendered, "no proof on this path shows value is string"); got != 2 {
		t.Fatalf("rendered evidence count = %d, want exact duplicate suppressed but distinct span kept:\n%s", got, rendered)
	}
}

func TestRenderDiagnosticPrintsRepeatedEvidenceFrameOnce(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 17},
		Span:     Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 20},
		Code:     Code("type.assignment"),
		Message:  "cannot assign value to string",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				File:    "provider.lua",
				Span:    Span{StartLine: 2, StartCol: 12, EndLine: 2, EndCol: 19},
				Message: "loader returns string?",
			},
			Evidence{
				Kind:    EvidencePrecisionBoundary,
				Trust:   TrustUnknown,
				File:    "provider.lua",
				Span:    Span{StartLine: 2, StartCol: 12, EndLine: 2, EndCol: 19},
				Message: "the indexed result may be absent",
			},
		),
		Labels: []Label{{Span: Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 20}, Message: "assigned value"}},
	}, RenderOptions{Sources: SourceMap{
		"main.lua":     "local name: string = raw",
		"provider.lua": "function load(id)\n    return rows[id]\nend",
	}})

	containsAll(t, rendered,
		"1. proven: loader returns string?",
		"2. unvalidated value: the indexed result may be absent",
		"--> provider.lua:2:12",
		"2 |     return rows[id]",
	)
	if got := strings.Count(rendered, "--> provider.lua:2:12"); got != 1 {
		t.Fatalf("shared evidence source frame should render once, got %d copies:\n%s", got, rendered)
	}
}

func TestRenderDiagnosticSuppressesEvidenceFrameCoveredByPrimary(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 22},
		Span:     Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 52},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nested read because it may be nil",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				Span:    Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 52},
				Message: "provider.get(...).tags[\"source\"] has type string? (may be nil)",
			},
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				Span:    Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 44},
				Message: "provider.get(...).tags may be nil before reading [\"source\"]",
			},
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				Span:    Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 39},
				Message: "provider.get(...) may be nil before reading .tags",
			},
		),
		Labels: []Label{{Span: Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 52}, Message: "assigned value"}},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": `local source: string = provider.get("a").tags["source"]`},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"1. proven: provider.get(...).tags[\"source\"] has type string? (may be nil)",
		"2. proven: provider.get(...).tags may be nil before reading [\"source\"]",
		"3. proven: provider.get(...) may be nil before reading .tags",
		"  |                      ↑ assigned value",
	)
	if got := strings.Count(rendered, "--> main.lua:1:22"); got != 1 {
		t.Fatalf("covered evidence spans should not repeat the primary source frame, got %d copies:\n%s", got, rendered)
	}
}

func TestRenderDiagnosticAttachesLabelsToEvidenceFrames(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 17},
		Span:     Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 27},
		Code:     Code("type.assignment"),
		Message:  "cannot assign provider.value because it may be nil",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				Span:    Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 27},
				Message: "provider.value has type string? (may be nil)",
			},
			Evidence{
				Kind:    EvidenceUserAssertion,
				Trust:   TrustClaimed,
				Span:    Span{StartLine: 2, StartCol: 13, EndLine: 2, EndCol: 19},
				Message: "target is declared as string",
			},
		),
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 27}, Message: "assigned value", Placement: LabelPlacementBelow},
			{Span: Span{StartLine: 2, StartCol: 13, EndLine: 2, EndCol: 19}, Message: "declared type", Placement: LabelPlacementAbove},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": "local target = provider.value\nlocal name: string = target"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"  |                 ↑ assigned value",
		"2 | local name: string = target",
		"  |             ↓ declared type",
	)
	if strings.Contains(rendered, "\nwhere:\n") {
		t.Fatalf("labels already attached to source frames should not render a trailing labels section:\n%s", rendered)
	}
}

func TestRenderDiagnosticSuppressesSameLineEvidenceFrame(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 27},
		Span:     Span{StartLine: 1, StartCol: 27, EndLine: 1, EndCol: 34},
		Code:     Code("type.call.direct.result_assignment"),
		Message:  "call result 1 is string, not number",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				Span:    Span{StartLine: 1, StartCol: 27, EndLine: 1, EndCol: 34},
				Message: "load_name returns string",
			},
			Evidence{
				Kind:    EvidenceUserAssertion,
				Trust:   TrustClaimed,
				Span:    Span{StartLine: 1, StartCol: 18, EndLine: 1, EndCol: 24},
				Message: "assignment target expects number",
			},
		),
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 18, EndLine: 1, EndCol: 24}, Message: "declared type"},
			{Span: Span{StartLine: 1, StartCol: 27, EndLine: 1, EndCol: 34}, Message: "call result"},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": "local got_count: number = load_name()"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"1 | local got_count: number = load_name()",
		"  |                  ↓ declared type",
		"  |                           ↑ call result",
		"1. proven: load_name returns string",
		"2. claimed: assignment target expects number",
	)
	if got := strings.Count(rendered, "local got_count: number = load_name()"); got != 1 {
		t.Fatalf("same-line evidence should not repeat the source line, got %d copies:\n%s", got, rendered)
	}
	if strings.Contains(rendered, "--> main.lua:1:18") {
		t.Fatalf("same-line evidence frame should be suppressed after the primary frame:\n%s", rendered)
	}
	if strings.Contains(rendered, "\n  |                  ^        ^") ||
		strings.Contains(rendered, "\n  = declared type; call result") ||
		strings.Contains(rendered, "\n  |    declared type ^        ^ call result") {
		t.Fatalf("wide same-line labels should render as one clean label layer:\n%s", rendered)
	}
}

func TestRenderDiagnosticCombinesMultipleLabelsOnSameFrame(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 21},
		Span:     Span{StartLine: 1, StartCol: 21, EndLine: 1, EndCol: 29},
		Code:     Code("type.assignment"),
		Message:  "cannot assign raw.name to string",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 21, EndLine: 1, EndCol: 29}, Message: "assigned value"},
			{Span: Span{StartLine: 1, StartCol: 21, EndLine: 1, EndCol: 29}, Message: "may be nil"},
			{Span: Span{StartLine: 1, StartCol: 21, EndLine: 1, EndCol: 29}, Message: "assigned value"},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": "local name: string = raw.name"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"  |                     ↑ assigned value; may be nil",
	)
	if strings.Contains(rendered, "\nwhere:\n") {
		t.Fatalf("primary labels should be attached to the primary source frame:\n%s", rendered)
	}
	if got := strings.Count(rendered, "assigned value"); got != 1 {
		t.Fatalf("duplicate primary label should be suppressed, got %d copies:\n%s", got, rendered)
	}
}

func TestRenderDiagnosticSeparatesNestedSameLineLabelRows(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 23},
		Span:     Span{StartLine: 1, StartCol: 23, EndLine: 1, EndCol: 44},
		Code:     Code("type.assignment"),
		Message:  "cannot assign provider result to string",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 13, EndLine: 1, EndCol: 19}, Message: "declared type", Placement: LabelPlacementAbove},
			{Span: Span{StartLine: 1, StartCol: 23, EndLine: 1, EndCol: 44}, Message: "assigned value", Placement: LabelPlacementBelow},
			{Span: Span{StartLine: 1, StartCol: 32, EndLine: 1, EndCol: 39}, Message: "nilable field", Placement: LabelPlacementBelow},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": "local name: string = provider.get().field"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"  |             ↓ declared type",
		"1 | local name: string = provider.get().field",
		"  |                       ↑ assigned value",
		"  |                                ↑ nilable field",
	)
	if strings.Contains(rendered, "\nwhere:\n") {
		t.Fatalf("same-line nested labels should attach to the source frame:\n%s", rendered)
	}
	for _, label := range []string{"declared type", "assigned value", "nilable field"} {
		if got := strings.Count(rendered, label); got != 1 {
			t.Fatalf("label %q rendered %d times, want once:\n%s", label, got, rendered)
		}
	}
	if strings.Contains(rendered, "↑ assigned value ↑ nilable field") {
		t.Fatalf("overlapping below labels should not be squeezed onto one row:\n%s", rendered)
	}
}

func TestRenderDiagnosticDenseSameLineLabelsDoNotOverlap(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 15},
		Span:     Span{StartLine: 1, StartCol: 15, EndLine: 1, EndCol: 18},
		Code:     Code("type.assignment"),
		Message:  "cannot assign dense expression",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 15, EndLine: 1, EndCol: 18}, Message: "first value", Placement: LabelPlacementBelow},
			{Span: Span{StartLine: 1, StartCol: 18, EndLine: 1, EndCol: 21}, Message: "second value", Placement: LabelPlacementBelow},
			{Span: Span{StartLine: 1, StartCol: 21, EndLine: 1, EndCol: 24}, Message: "third value", Placement: LabelPlacementBelow},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": "local result = one + two + three"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"1 | local result = one + two + three",
		"↑ first value",
		"↑ second value",
		"↑ third value",
	)
	if strings.Contains(rendered, "\nwhere:\n") {
		t.Fatalf("dense same-line labels should attach to the source frame:\n%s", rendered)
	}
	for _, label := range []string{"first value", "second value", "third value"} {
		if got := strings.Count(rendered, label); got != 1 {
			t.Fatalf("label %q rendered %d times, want once:\n%s", label, got, rendered)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "↑") && strings.Count(line, "↑") != 1 {
			t.Fatalf("dense annotation row has overlapping arrows:\n%s\n\nfull render:\n%s", line, rendered)
		}
	}
	if arrowRows := strings.Count(rendered, "↑ first value") +
		strings.Count(rendered, "↑ second value") +
		strings.Count(rendered, "↑ third value"); arrowRows != 3 {
		t.Fatalf("dense labels should each get a visible arrow row, got %d:\n%s", arrowRows, rendered)
	}
}

func TestRenderDiagnosticUTF8SourceKeepsLabelRowsAligned(t *testing.T) {
	source := "local caf\u00e9: string = raw"
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 22},
		Span:     Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 25},
		Code:     Code("type.assignment"),
		Message:  "cannot assign raw",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 13, EndLine: 1, EndCol: 19}, Message: "declared type", Placement: LabelPlacementAbove},
			{Span: Span{StartLine: 1, StartCol: 22, EndLine: 1, EndCol: 25}, Message: "assigned value", Placement: LabelPlacementBelow},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": source},
		ShowSourceLabelRows: true,
	})

	want := `error[type.assignment]: cannot assign raw
 --> main.lua:1:22
  |
  |             ↓ declared type
1 | local café: string = raw
  |                      ↑ assigned value`
	assertRenderedEqual(t, rendered, want)
}

func TestRenderDiagnosticOmitsUnlabeledPrimaryWhenSameLineLabelsExist(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 17},
		Span:     Span{StartLine: 1, StartCol: 17, EndLine: 1, EndCol: 24},
		Code:     Code("type.call.direct.argument_type"),
		Message:  "cannot pass store.state.last_tick as argument 1 because it may be nil",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 25, EndLine: 1, EndCol: 46}, Message: "argument value", Placement: LabelPlacementBelow},
		},
	}, RenderOptions{
		Sources:             SourceMap{"main.lua": "local elapsed = now:sub(store.state.last_tick)"},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"1 | local elapsed = now:sub(store.state.last_tick)",
		"  |                         ↑ argument value",
	)
	if strings.Contains(rendered, "  |                 ^\n") {
		t.Fatalf("unlabeled primary marker should not compete with labeled same-line spans:\n%s", rendered)
	}
}

func TestRenderDiagnosticAmbiguousFilelessLabelFallsBackToText(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
		Labels:   []Label{{Span: Span{StartLine: 1, StartCol: 7}, Message: "assigned value"}},
	}, RenderOptions{Sources: SourceMap{
		"main.lua":     "local x = nil",
		"protocol.lua": "type X = {}",
	}})

	containsAll(t, rendered, "where:", "line 1:7: assigned value")
	if strings.Contains(rendered, "--> <unknown>") {
		t.Fatalf("ambiguous fileless label should not render unknown file frame:\n%s", rendered)
	}
}

func TestRenderDiagnosticTrimsCRLFSourceLines(t *testing.T) {
	rendered := Render(Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
	}, RenderOptions{Sources: SourceMap{"main.lua": "local x = nil\r\nlocal y = 1\r\n"}})

	containsAll(t, rendered, "1 | local x = nil")
	if strings.Contains(rendered, "\r") {
		t.Fatalf("rendered diagnostic leaked CRLF carriage return:\n%q", rendered)
	}
}

func TestRenderDiagnosticClampsWideColumns(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 100},
		Code:     Code("parse"),
		Message:  "bad token",
		Severity: SeverityError,
	}

	rendered := Render(d, RenderOptions{Sources: SourceMap{"main.lua": "short"}})
	containsAll(t, rendered, "1 | short", "     ^")
}

func TestRenderDiagnosticWithoutFileUsesSingleSourceLocation(t *testing.T) {
	d := Diagnostic{
		Position: Position{Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
	}

	rendered := Render(d, RenderOptions{Sources: SourceMap{"only.lua": "local x = nil"}})
	containsAll(t, rendered, "--> only.lua:1:7", "1 | local x = nil", "      ^")
	if strings.Contains(rendered, "test.lua") {
		t.Fatalf("rendered diagnostic leaked fixture filename:\n%s", rendered)
	}
}

func TestRenderDiagnosticWithoutFileAndAmbiguousSourcesOmitsFrame(t *testing.T) {
	d := Diagnostic{
		Position: Position{Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
	}

	rendered := Render(d, RenderOptions{Sources: SourceMap{
		"main.lua":     "local x = nil",
		"protocol.lua": "type X = {}",
	}})
	if strings.Contains(rendered, "-->") || strings.Contains(rendered, "<unknown>") {
		t.Fatalf("ambiguous fileless diagnostic should not render a misleading frame:\n%s", rendered)
	}
	containsAll(t, rendered, "error[type.assignment]: cannot assign nil to string")
}

func TestRenderDiagnosticUsesDisplayFileAlias(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "test.lua", Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
	}

	rendered := Render(d, RenderOptions{
		Sources:      SourceMap{"main.lua": "local x = nil"},
		DisplayFiles: map[string]string{"test.lua": "main.lua"},
	})
	containsAll(t, rendered, "--> main.lua:1:7", "1 | local x = nil")
	if strings.Contains(rendered, "--> test.lua") {
		t.Fatalf("rendered diagnostic ignored display alias:\n%s", rendered)
	}
}

func TestRenderDiagnosticUsesDisplayFileAliasForSpanlessLabels(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "test.lua", Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
		Labels:   []Label{{File: "test.lua", Message: "entry point"}},
	}

	rendered := Render(d, RenderOptions{
		Sources:      SourceMap{"main.lua": "local x = nil"},
		DisplayFiles: map[string]string{"test.lua": "main.lua"},
	})
	containsAll(t, rendered, "--> main.lua", "= entry point")
	if strings.Contains(rendered, "--> test.lua") {
		t.Fatalf("spanless label ignored display alias:\n%s", rendered)
	}
}

func TestRenderDiagnosticOmitsImplicitSpanlessLabels(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 7},
		Span:     Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8}, Message: "assigned value"},
			{Message: "declared type"},
		},
	}

	rendered := Render(d, RenderOptions{
		Sources:             SourceMap{"main.lua": "local x = nil"},
		ShowSourceLabelRows: true,
	})
	if strings.Contains(rendered, "\nwhere:\n") || strings.Contains(rendered, "= declared type") {
		t.Fatalf("rendered implicit spanless label fallback:\n%s", rendered)
	}
	containsAll(t, rendered, "--> main.lua:1:7", "1 | local x = nil", "  |       ↑ assigned value")
}

func TestRenderDiagnosticOmitsStructuralEmptyLabels(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 7},
		Span:     Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 8}, Message: "assigned value"},
			{Span: Span{StartLine: 1, StartCol: 15, EndLine: 1, EndCol: 18}},
		},
	}

	rendered := Render(d, RenderOptions{
		Sources:             SourceMap{"main.lua": "local x = nil"},
		ShowSourceLabelRows: true,
	})
	if strings.Contains(rendered, "\nwhere:\n") || strings.Contains(rendered, "^") {
		t.Fatalf("rendered structural empty label:\n%s", rendered)
	}
	containsAll(t, rendered, "--> main.lua:1:7", "1 | local x = nil", "  |       ↑ assigned value")
}

func TestRenderDiagnosticCrossFileEvidenceChain(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 3, Column: 33},
		Span:     Span{StartLine: 3, StartCol: 33, EndLine: 3, EndCol: 47},
		Code:     Code("type.assignment"),
		Message:  "cached.content may be nil here",
		Severity: SeverityError,
		Explanation: NewExplanation(
			Evidence{
				Kind:    EvidenceAbstractFact,
				Trust:   TrustProven,
				File:    "session_store.lua",
				Span:    Span{StartLine: 2, StartCol: 12, EndLine: 2, EndCol: 29},
				Message: "cache lookup can miss and returns Receipt?",
			},
			Evidence{
				Kind:    EvidenceUserAssertion,
				Trust:   TrustClaimed,
				File:    "protocol.lua",
				Span:    Span{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 19},
				Message: "Receipt.content is declared string?",
			},
			Evidence{
				Kind:    EvidenceMissingProof,
				Trust:   TrustUnknown,
				Span:    Span{StartLine: 3, StartCol: 33, EndLine: 3, EndCol: 47},
				Message: "no dominating guard proves cached.content ~= nil",
			},
		),
		Labels: []Label{
			{File: "protocol.lua", Span: Span{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 19}, Message: "optional field", Placement: LabelPlacementAbove},
			{Span: Span{StartLine: 3, StartCol: 24, EndLine: 3, EndCol: 30}, Message: "requires string", Placement: LabelPlacementAbove},
			{Span: Span{StartLine: 3, StartCol: 33, EndLine: 3, EndCol: 47}, Message: "value is string?", Placement: LabelPlacementBelow},
		},
		Help: "guard cached.content before assigning it to a string.",
	}

	rendered := Render(d, RenderOptions{
		Sources: SourceMap{
			"main.lua":          "local cached = store:get(id)\nif cached ~= nil then\n    local bad_content: string = cached.content\nend",
			"session_store.lua": "function Store:get(id)\n    return self.receipts[id]\nend",
			"protocol.lua":      "type Receipt = {\n  content: string?,\n}",
		},
		ShowSourceLabelRows: true,
	})

	containsAll(t, rendered,
		"error[type.assignment]: cached.content may be nil here",
		"--> main.lua:3:33",
		"3 |     local bad_content: string = cached.content",
		"because:",
		"1. proven: cache lookup can miss and returns Receipt?",
		"--> session_store.lua:2:12",
		"2 |     return self.receipts[id]",
		"2. claimed: Receipt.content is declared string?",
		"--> protocol.lua:2:3",
		"2 |   content: string?,",
		"3. missing proof: no dominating guard proves cached.content ~= nil",
		"optional field",
		"requires string",
		"value is string?",
		"help: guard cached.content before assigning it to a string.",
	)
	if got := strings.Count(rendered, "--> main.lua:3:33"); got != 1 {
		t.Fatalf("primary source frame should render once, got %d copies:\n%s", got, rendered)
	}
	if got := strings.Count(rendered, "--> protocol.lua:2:3"); got != 1 {
		t.Fatalf("protocol source frame should render once, got %d copies:\n%s", got, rendered)
	}

	want := `error[type.assignment]: cached.content may be nil here
 --> main.lua:3:33
  |
  |                        ↓ requires string
3 |     local bad_content: string = cached.content
  |                                 ↑ value is string?

because:
  1. proven: cache lookup can miss and returns Receipt?
 --> session_store.lua:2:12
  |
2 |     return self.receipts[id]
  |            ^
  2. claimed: Receipt.content is declared string?
 --> protocol.lua:2:3
  |
  |   ↓ optional field
2 |   content: string?,
  3. missing proof: no dominating guard proves cached.content ~= nil

help: guard cached.content before assigning it to a string.`
	assertRenderedEqual(t, rendered, want)
}

func TestRenderDiagnosticUsesAnsiColorWhenRequested(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 7},
		Code:     Code("type.assignment"),
		Message:  "cannot assign nil to string",
		Severity: SeverityError,
		Explanation: NewExplanation(Evidence{
			Kind:    EvidenceMissingProof,
			Trust:   TrustUnknown,
			Span:    Span{StartLine: 1, StartCol: 7},
			Message: "no proof on this path shows this value is string",
		}),
		Help: "check for nil before assignment.",
	}

	rendered := Render(d, RenderOptions{
		Sources: SourceMap{"main.lua": "local x = nil"},
		Color:   true,
	})
	containsAll(t, rendered,
		"\x1b[1;31merror\x1b[0m[type.assignment]",
		"\x1b[34m-->\x1b[0m main.lua:1:7",
		"\x1b[1;33m^\x1b[0m",
		"\x1b[1;31mmissing proof\x1b[0m",
		"\x1b[1;32mcheck for nil before assignment.\x1b[0m",
	)

	want := "\x1b[1;31merror\x1b[0m[type.assignment]: \x1b[1mcannot assign nil to string\x1b[0m\n" +
		" \x1b[34m-->\x1b[0m main.lua:1:7\n" +
		"  \x1b[34m|\x1b[0m\n" +
		"1 \x1b[34m|\x1b[0m local x = nil\n" +
		"  \x1b[34m|\x1b[0m       \x1b[1;33m^\x1b[0m\n\n" +
		"\x1b[34mbecause\x1b[0m:\n" +
		"  1. \x1b[1;31mmissing proof\x1b[0m: no proof on this path shows this value is string\n\n" +
		"help: \x1b[1;32mcheck for nil before assignment.\x1b[0m"
	assertRenderedEqual(t, rendered, want)
}

func TestRenderDiagnosticExpandsTabsForAlignedSourceFrames(t *testing.T) {
	d := Diagnostic{
		Position: Position{File: "main.lua", Line: 1, Column: 12},
		Span:     Span{StartLine: 1, StartCol: 12, EndLine: 1, EndCol: 18},
		Code:     Code("type.assignment"),
		Message:  "cannot pass source",
		Severity: SeverityError,
		Labels: []Label{
			{Span: Span{StartLine: 1, StartCol: 12, EndLine: 1, EndCol: 18}, Message: "argument value"},
		},
	}

	rendered := Render(d, RenderOptions{
		Sources:             SourceMap{"main.lua": "\tchannel = source.primary"},
		ShowSourceLabelRows: true,
	})

	want := `error[type.assignment]: cannot pass source
 --> main.lua:1:12
  |
1 |     channel = source.primary
  |               ↑ argument value`
	assertRenderedEqual(t, rendered, want)
	if strings.Contains(rendered, "\t") {
		t.Fatalf("rendered source frame should expand tabs for stable caret alignment:\n%q", rendered)
	}
}

func assertRenderedEqual(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", lineDiff(want, got))
}

func lineDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	n := len(wantLines)
	if len(gotLines) > n {
		n = len(gotLines)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		var wantLine, gotLine string
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if wantLine == gotLine {
			continue
		}
		if i < len(wantLines) {
			b.WriteString("- ")
			b.WriteString(wantLine)
			b.WriteString("\n")
		}
		if i < len(gotLines) {
			b.WriteString("+ ")
			b.WriteString(gotLine)
			b.WriteString("\n")
		}
	}
	if b.Len() == 0 {
		return "line content matched, but rendered strings differ"
	}
	return b.String()
}

func containsAll(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", needle, haystack)
		}
	}
}
