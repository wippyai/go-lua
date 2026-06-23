package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestOptionalExhaustivenessReportsConsumedValueWithoutNilCase(t *testing.T) {
	src := strings.TrimLeft(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        sink.seen = maybe
    end
    return sink.seen
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            4,
		Column:          8,
		MessageContains: []string{
			"optional handling is not exhaustive",
			"maybe == nil",
		},
		EvidenceMin: 5,
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"possible cases: `maybe ~= nil`, `maybe == nil`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
			"no else branch handles the remaining optional case",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch checks optional `maybe`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`maybe ~= nil`", "`maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"consumed case", "`maybe ~= nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases", "`maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no else branch handles the remaining optional case"},
			},
		},
		LabelContains: []string{"optional case check"},
		HelpContains:  []string{"Handle the nil case"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: optional handling is not exhaustive; missing case: ` + "`maybe == nil`" + `
 --> test.lua:4:8
  |
4 |     if maybe ~= nil then
  |        ↑ optional case check

because:
  1. proven: branch checks optional ` + "`maybe`" + `
  2. proven: possible cases: ` + "`maybe ~= nil`, `maybe == nil`" + `
  3. proven: consumed case: ` + "`maybe ~= nil`" + `
  4. missing proof: missing cases: ` + "`maybe == nil`" + `
  5. missing proof: no else branch handles the remaining optional case

help: Handle the nil case with an else branch, or return before continuing when nil is intentionally ignored.`
	assertRenderedEqual(t, rendered, want)
}

func TestOptionalExhaustivenessDoesNotUseInvalidatedValueProof(t *testing.T) {
	result := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        maybe = nil
        sink.seen = maybe
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after reassignment invalidates value proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessReportsAliasConsumedBeforeOriginalInvalidated(t *testing.T) {
	src := strings.TrimLeft(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        local alias = maybe
        maybe = nil
        sink.seen = alias
    end
    return sink.seen
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            4,
		Column:          8,
		MessageContains: []string{"optional handling is not exhaustive", "maybe == nil"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch checks optional `maybe`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases: `maybe ~= nil`, `maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"consumed case: `maybe ~= nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases: `maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no else branch handles the remaining optional case"},
			},
		},
		LabelContains: []string{"optional case check"},
		HelpContains:  []string{"Handle the nil case", "return before continuing"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: optional handling is not exhaustive; missing case: ` + "`maybe == nil`" + `
 --> test.lua:4:8
  |
4 |     if maybe ~= nil then
  |        ↑ optional case check

because:
  1. proven: branch checks optional ` + "`maybe`" + `
  2. proven: possible cases: ` + "`maybe ~= nil`, `maybe == nil`" + `
  3. proven: consumed case: ` + "`maybe ~= nil`" + `
  4. missing proof: missing cases: ` + "`maybe == nil`" + `
  5. missing proof: no else branch handles the remaining optional case

help: Handle the nil case with an else branch, or return before continuing when nil is intentionally ignored.`
	assertRenderedEqual(t, rendered, want)
}

func TestOptionalExhaustivenessReportsOriginalConsumedThroughAliasGuard(t *testing.T) {
	src := strings.TrimLeft(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    local alias = maybe
    if alias ~= nil then
        sink.seen = maybe
    end
    return sink.seen
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		Line:            5,
		Column:          8,
		MessageContains: []string{"optional handling is not exhaustive", "alias == nil"},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch checks optional `alias`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases: `alias ~= nil`, `alias == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"consumed case: `alias ~= nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases: `alias == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no else branch handles the remaining optional case"},
			},
		},
		LabelContains: []string{"optional case check"},
		HelpContains:  []string{"Handle the nil case", "return before continuing"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: optional handling is not exhaustive; missing case: ` + "`alias == nil`" + `
 --> test.lua:5:8
  |
5 |     if alias ~= nil then
  |        ↑ optional case check

because:
  1. proven: branch checks optional ` + "`alias`" + `
  2. proven: possible cases: ` + "`alias ~= nil`, `alias == nil`" + `
  3. proven: consumed case: ` + "`alias ~= nil`" + `
  4. missing proof: missing cases: ` + "`alias == nil`" + `
  5. missing proof: no else branch handles the remaining optional case

help: Handle the nil case with an else branch, or return before continuing when nil is intentionally ignored.`
	assertRenderedEqual(t, rendered, want)
}

func TestOptionalExhaustivenessReportsAliasConsumedThroughOriginalGuard(t *testing.T) {
	src := strings.TrimLeft(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    local alias = maybe
    if maybe ~= nil then
        sink.seen = alias
    end
    return sink.seen
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	diag := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            5,
		Column:          8,
		MessageContains: []string{"optional handling is not exhaustive", "maybe == nil"},
		EvidenceMin:     5,
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch checks optional `maybe`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases: `maybe ~= nil`, `maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"consumed case: `maybe ~= nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases: `maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no else branch handles the remaining optional case"},
			},
		},
		LabelContains: []string{"optional case check"},
		HelpContains:  []string{"Handle the nil case", "return before continuing"},
	})
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `warning[lint.union.exhaustiveness]: optional handling is not exhaustive; missing case: ` + "`maybe == nil`" + `
 --> test.lua:5:8
  |
5 |     if maybe ~= nil then
  |        ↑ optional case check

because:
  1. proven: branch checks optional ` + "`maybe`" + `
  2. proven: possible cases: ` + "`maybe ~= nil`, `maybe == nil`" + `
  3. proven: consumed case: ` + "`maybe ~= nil`" + `
  4. missing proof: missing cases: ` + "`maybe == nil`" + `
  5. missing proof: no else branch handles the remaining optional case

help: Handle the nil case with an else branch, or return before continuing when nil is intentionally ignored.`
	assertRenderedEqual(t, rendered, want)
}

func TestOptionalExhaustivenessDoesNotUseReassignedAliasGuardForOriginal(t *testing.T) {
	result := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, replacement: string?, sink: Sink): string
    local alias = maybe
    alias = replacement
    if alias ~= nil then
        sink.seen = maybe
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after alias reassignment breaks equivalence", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotUseInvalidatedFieldProof(t *testing.T) {
	result := Check(`
type Box = { value: string? }
type Sink = { seen: string }

local function remember(box: Box, sink: Sink): string
    if box.value ~= nil then
        box.value = nil
        sink.seen = box.value
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after field reassignment invalidates value proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotUseDynamicIndexInvalidatedFieldProof(t *testing.T) {
	result := Check(`
type Box = { value: string? }
type Sink = { seen: string }

local function remember(box: Box, key: string, sink: Sink): string
    if box.value ~= nil then
        box[key] = nil
        sink.seen = box.value
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after dynamic index write invalidates field proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotUseCallInvalidatedFieldProof(t *testing.T) {
	result := Check(`
type Box = { value: string? }
type Sink = { seen: string }

local function clear(box: Box, key: string): ()
    box[key] = nil
end

local function remember(box: Box, sink: Sink): string
    if box.value ~= nil then
        clear(box, "value")
        sink.seen = box.value
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after call invalidates field proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotUseAllBranchesInvalidatedValueProof(t *testing.T) {
	result := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, flag: boolean, sink: Sink): string
    if maybe ~= nil then
        if flag then
            maybe = nil
        else
            maybe = nil
        end
        sink.seen = maybe
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after every branch invalidates value proof", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessReportsReachableBranchConsumptionBeforeInvalidation(t *testing.T) {
	src := strings.TrimLeft(`
type Sink = { seen: string }

local function remember(maybe: string?, flag: boolean, sink: Sink): string
    if maybe ~= nil then
        if flag then
            sink.seen = maybe
        else
            maybe = nil
        end
    end
    return sink.seen
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            4,
		Column:          8,
		MessageContains: []string{"optional handling is not exhaustive", "maybe == nil"},
		EvidenceMin:     5,
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"possible cases: `maybe ~= nil`, `maybe == nil`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
			"no else branch handles the remaining optional case",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch checks optional `maybe`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`maybe ~= nil`", "`maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"consumed case", "`maybe ~= nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases", "`maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no else branch handles"},
			},
		},
		LabelContains: []string{"optional case check"},
		HelpContains:  []string{"Handle the nil case"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: optional handling is not exhaustive",
			"--> test.lua:4:8",
			"4 |     if maybe ~= nil then",
			"↑ optional case check",
			"because:",
			"1. proven: branch checks optional `maybe`",
			"2. proven: possible cases: `maybe ~= nil`, `maybe == nil`",
			"3. proven: consumed case: `maybe ~= nil`",
			"4. missing proof: missing cases: `maybe == nil`",
			"5. missing proof: no else branch handles the remaining optional case",
			"help: Handle the nil case",
		},
		RenderNotContains: []string{
			"maybe == false",
		},
	})
}

func TestOptionalExhaustivenessAcceptsExplicitNilHandlingAndGuardReturn(t *testing.T) {
	withElse := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe ~= nil then
        sink.seen = maybe
    else
        sink.seen = "missing"
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(withElse.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning with else fallback", withElse.Diagnostics)
	}

	guardReturn := Check(`
local function value_or_default(maybe: string?): string
    if maybe ~= nil then
        return maybe
    end
    return "fallback"
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(guardReturn.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning for guard-return fallback", guardReturn.Diagnostics)
	}

	nilGuard := Check(`
local function value_or_default(maybe: string?): string
    if maybe == nil then
        return "fallback"
    end
    return maybe
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(nilGuard.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning after nil guard", nilGuard.Diagnostics)
	}
}

func TestOptionalExhaustivenessAcceptsErrorTerminatedValueBranch(t *testing.T) {
	result := Check(`
local function value_or_default(maybe: string?): string
    if maybe ~= nil then
        error(maybe)
    end
    return "fallback"
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning when value branch raises", result.Diagnostics)
	}
}

func TestOptionalExhaustivenessDoesNotTreatShadowedErrorAsTerminating(t *testing.T) {
	src := strings.TrimLeft(`
local function value_or_default(maybe: string?): string
    local function error(message: string): () end
    if maybe ~= nil then
        error(maybe)
    end
    return "fallback"
end
`, "\n")
	result := Check(src, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		Line:            3,
		Column:          8,
		MessageContains: []string{"optional handling is not exhaustive", "maybe == nil"},
		EvidenceMin:     5,
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"possible cases: `maybe ~= nil`, `maybe == nil`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
			"no else branch handles the remaining optional case",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"branch checks optional `maybe`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"possible cases", "`maybe ~= nil`", "`maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"consumed case", "`maybe ~= nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"missing cases", "`maybe == nil`"},
			},
			{
				Kind:            diagnostic.EvidenceMissingProof,
				Trust:           diagnostic.TrustUnknown,
				MessageContains: []string{"no else branch handles"},
			},
		},
		LabelContains: []string{"optional case check"},
		HelpContains:  []string{"Handle the nil case"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"warning[lint.union.exhaustiveness]: optional handling is not exhaustive",
			"--> test.lua:3:8",
			"3 |     if maybe ~= nil then",
			"↑ optional case check",
			"because:",
			"1. proven: branch checks optional `maybe`",
			"2. proven: possible cases: `maybe ~= nil`, `maybe == nil`",
			"3. proven: consumed case: `maybe ~= nil`",
			"4. missing proof: missing cases: `maybe == nil`",
			"5. missing proof: no else branch handles the remaining optional case",
			"help: Handle the nil case",
		},
		RenderNotContains: []string{
			"global error",
		},
	})
}

func TestOptionalExhaustivenessHandlesTruthyOptionalWithoutBooleanFalsePositive(t *testing.T) {
	stringOptional := Check(`
type Sink = { seen: string }

local function remember(maybe: string?, sink: Sink): string
    if maybe then
        sink.seen = maybe
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, stringOptional, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{
			"optional handling is not exhaustive",
			"maybe == nil",
		},
		EvidenceOrdered: []string{
			"branch checks optional `maybe`",
			"consumed case: `maybe ~= nil`",
			"missing cases: `maybe == nil`",
		},
	})

	booleanOptional := Check(`
type Sink = { seen: string }

local function remember(flag: boolean?, sink: Sink): string
    if flag then
        sink.seen = "true"
    end
    return sink.seen
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(booleanOptional.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no optional exhaustiveness warning when false is a non-nil value", booleanOptional.Diagnostics)
	}
}
