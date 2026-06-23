package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestInterprocRelationalIndexHelperProvesCallerArrayRead(t *testing.T) {
	cases := map[string]string{
		"bad-guard-sum-upper-bound": `
local function require_sum(xs: {number}, i: number, j: number): ()
    if i < 1 or j < 0 or i + j > #xs then
        error("oob")
    end
end

local function read(xs: {number}, i: number, j: number): number
    require_sum(xs, i, j)
    local n: number = xs[i]
    return n
end
`,
		"bad-guard-scaled-upper-bound": `
local function require_scaled(xs: {number}, i: number): ()
    if i < 1 or 2 * i > #xs then
        error("oob")
    end
end

local function read(xs: {number}, i: number): number
    require_scaled(xs, i)
    local n: number = xs[2 * i]
    return n
end
`,
		"positive-sum-upper-bound": `
local function require_sum(xs: {number}, i: number, j: number): ()
    if i >= 1 and j >= 0 and i + j <= #xs then
        return
    end
    error("oob")
end

local function read(xs: {number}, i: number, j: number): number
    require_sum(xs, i, j)
    local n: number = xs[i]
    return n
end
`,
		"positive-scaled-upper-bound": `
local function require_scaled(xs: {number}, i: number): ()
    if i >= 1 and 2 * i <= #xs then
        return
    end
    error("oob")
end

local function read(xs: {number}, i: number): number
    require_scaled(xs, i)
    local n: number = xs[2 * i]
    return n
end
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			result := Check(src, WithStdlib())
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want none: interproc relational guard should prove caller index read", result.Diagnostics)
			}
		})
	}
}

func TestInterprocRelationalIndexHelperStillRequiresSupportingFloors(t *testing.T) {
	cases := map[string]struct {
		src      string
		line     int
		column   int
		endCol   int
		readExpr string
		subject  string
	}{
		"sum-missing-second-nonnegative-floor": {
			src: strings.TrimLeft(`
local function require_sum(xs: {number}, i: number, j: number): ()
    if i < 1 or i + j > #xs then
        error("oob")
    end
end

local function read(xs: {number}, i: number, j: number): number
    require_sum(xs, i, j)
    local n: number = xs[i]
    return n
end
`, "\n"),
			line:     9,
			column:   23,
			endCol:   28,
			readExpr: "xs[i]",
			subject:  "xs[i]",
		},
		"scaled-missing-positive-floor": {
			src: strings.TrimLeft(`
local function require_scaled(xs: {number}, i: number): ()
    if 2 * i > #xs then
        error("oob")
    end
end

local function read(xs: {number}, i: number): number
    require_scaled(xs, i)
    local n: number = xs[2 * i]
    return n
end
`, "\n"),
			line:     9,
			column:   23,
			endCol:   32,
			readExpr: "xs[2 * i]",
			subject:  "xs",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result := Check(tc.src, WithStdlib())
			requireDiagnostic(t, result, diagnosticExpectation{
				Code:            diagnostics.CodeAssignmentType,
				DiagnosticCount: 1,
				Line:            tc.line,
				Column:          tc.column,
				Span: diagnostic.Span{
					StartLine: tc.line,
					StartCol:  tc.column,
					EndLine:   tc.line,
					EndCol:    tc.endCol,
				},
				MessageContains: []string{"cannot assign " + tc.subject, "may be nil"},
				EvidenceMin:     3,
				EvidenceContains: []string{
					tc.subject + " can be number or nil here",
					"n is declared as number",
					"indexed read that can miss or read nil",
					"no proof shows the selected slot satisfies the declared type here",
				},
				EvidenceOrdered: []string{
					tc.subject + " can be number or nil here",
					"n is declared as number",
					tc.subject + " is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here",
				},
				EvidenceChain: []diagnosticEvidenceExpectation{
					{
						Kind:            diagnostic.EvidenceAbstractFact,
						Trust:           diagnostic.TrustProven,
						MessageContains: []string{tc.subject, "number or nil"},
					},
					{
						Kind:            diagnostic.EvidenceUserAssertion,
						Trust:           diagnostic.TrustClaimed,
						MessageContains: []string{"n", "number"},
					},
					{
						Kind:            diagnostic.EvidenceMissingProof,
						Trust:           diagnostic.TrustUnknown,
						MessageContains: []string{"indexed read", "miss", "nil", "selected slot", "declared type"},
					},
				},
				LabelMin:      2,
				LabelContains: []string{"assigned value", "declared type"},
				HelpContains:  []string{"Guard `" + tc.subject + "`", "provide a default value", "change the target type"},
				Sources:       diagnostic.SourceMap{"test.lua": tc.src},
				RenderOrderedContains: []string{
					"error[type.assignment]: cannot assign " + tc.subject + " because it may be nil",
					fmt.Sprintf("test.lua:%d:%d", tc.line, tc.column),
					"declared type",
					fmt.Sprintf("%d |     local n: number = %s", tc.line, tc.readExpr),
					"assigned value",
					"because:",
					"proven: " + tc.subject + " can be number or nil here",
					"claimed: n is declared as number",
					"missing proof: " + tc.subject + " is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here",
					"help: Guard `" + tc.subject + "` with a nil check",
				},
				RenderNotContains: []string{
					"want string",
					"^~",
				},
			})
		})
	}
}
