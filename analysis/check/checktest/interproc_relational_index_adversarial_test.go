package checktest

import (
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
		src    string
		line   int
		column int
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
			line:   9,
			column: 23,
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
			line:   9,
			column: 23,
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
				MessageContains: []string{"cannot assign", "may be nil"},
				EvidenceContains: []string{
					"n is declared as number",
					"indexed read that can miss or read nil",
					"no proof shows the selected slot satisfies the declared type here",
				},
				RenderNotContains: []string{
					"^~",
				},
				Sources: diagnostic.SourceMap{"test.lua": tc.src},
			})
		})
	}
}
