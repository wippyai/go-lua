package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestInterprocIndexGuardHelperProvesCallerArrayRead(t *testing.T) {
	result := Check(`
local function require_index(xs: {number}, i: number): ()
    if i < 1 then
        error("index below array range")
    end
    if i > #xs then
        error("index above array range")
    end
end

local function read(xs: {number}, i: number): number
    require_index(xs, i)
    local n: number = xs[i]
    return n
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: helper normal-return facts should prove i >= 1 and i <= #xs", result.Diagnostics)
	}
}

func TestInterprocIndexGuardOrHelperProvesCallerArrayRead(t *testing.T) {
	result := Check(`
local function require_index(xs: {number}, i: number): ()
    if i < 1 or i > #xs then
        error("index outside array range")
    end
end

local function read(xs: {number}, i: number): number
    require_index(xs, i)
    local n: number = xs[i]
    return n
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: false edge of OR guard should prove i >= 1 and i <= #xs", result.Diagnostics)
	}
}

func TestInterprocIndexGuardNegatedConjunctionHelperProvesCallerArrayRead(t *testing.T) {
	result := Check(`
local function require_index(xs: {number}, i: number): ()
    if not (i >= 1 and i <= #xs) then
        error("index outside array range")
    end
end

local function read(xs: {number}, i: number): number
    require_index(xs, i)
    local n: number = xs[i]
    return n
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: false edge of negated conjunction should prove i >= 1 and i <= #xs", result.Diagnostics)
	}
}

func TestInterprocIndexGuardFlippedOrHelperProvesCallerArrayRead(t *testing.T) {
	result := Check(`
local function require_index(xs: {number}, i: number): ()
    if 1 > i or #xs < i then
        error("index outside array range")
    end
end

local function read(xs: {number}, i: number): number
    require_index(xs, i)
    local n: number = xs[i]
    return n
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: flipped OR guard should prove i >= 1 and i <= #xs", result.Diagnostics)
	}
}

func TestInterprocIndexGuardFlippedNegatedConjunctionHelperProvesCallerArrayRead(t *testing.T) {
	result := Check(`
local function require_index(xs: {number}, i: number): ()
    if not (1 <= i and #xs >= i) then
        error("index outside array range")
    end
end

local function read(xs: {number}, i: number): number
    require_index(xs, i)
    local n: number = xs[i]
    return n
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none: flipped negated conjunction should prove i >= 1 and i <= #xs", result.Diagnostics)
	}
}

func TestInterprocIndexGuardDoesNotProjectReassignedParameterFloor(t *testing.T) {
	src := strings.TrimLeft(`
local function require_index(xs: {number}, i: number): ()
    i = 1
    if i > #xs then
        error("index above array range")
    end
end

local function read(xs: {number}, i: number): number
    require_index(xs, i)
    local n: number = xs[i]
    return n
end
`, "\n")
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            10,
		Column:          23,
		Span: diagnostic.Span{
			StartLine: 10,
			StartCol:  23,
			EndLine:   10,
			EndCol:    28,
		},
		MessageContains: []string{"cannot assign xs[i]", "may be nil"},
		EvidenceMin:     3,
		EvidenceContains: []string{
			"xs[i] can be number or nil here",
			"n is declared as number",
			"xs[i] is an indexed read that can miss or read nil",
			"no proof shows the selected slot satisfies the declared type here",
		},
		EvidenceOrdered: []string{
			"xs[i] can be number or nil here",
			"n is declared as number",
			"xs[i] is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"xs[i]", "number or nil"},
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
		HelpContains:  []string{"Guard `xs[i]`", "provide a default value", "change the target type"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign xs[i] because it may be nil",
			"test.lua:10:23",
			"declared type",
			"10 |     local n: number = xs[i]",
			"assigned value",
			"because:",
			"proven: xs[i] can be number or nil here",
			"claimed: n is declared as number",
			"missing proof: xs[i] is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here",
			"help: Guard `xs[i]` with a nil check",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}

func TestInterprocIndexGuardStillRequiresPositiveFloor(t *testing.T) {
	src := strings.TrimLeft(`
local function require_upper_bound(xs: {number}, i: number): ()
    if i > #xs then
        error("index above array range")
    end
end

local function read(xs: {number}, i: number): number
    require_upper_bound(xs, i)
    local n: number = xs[i]
    return n
end
`, "\n")
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            9,
		Column:          23,
		Span: diagnostic.Span{
			StartLine: 9,
			StartCol:  23,
			EndLine:   9,
			EndCol:    28,
		},
		MessageContains: []string{"cannot assign xs[i]", "may be nil"},
		EvidenceMin:     3,
		EvidenceContains: []string{
			"xs[i] can be number or nil here",
			"n is declared as number",
			"xs[i] is an indexed read that can miss or read nil",
			"no proof shows the selected slot satisfies the declared type here",
		},
		EvidenceOrdered: []string{
			"xs[i] can be number or nil here",
			"n is declared as number",
			"xs[i] is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"xs[i]", "number or nil"},
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
		HelpContains:  []string{"Guard `xs[i]`", "provide a default value", "change the target type"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign xs[i] because it may be nil",
			"test.lua:9:23",
			"declared type",
			"9 |     local n: number = xs[i]",
			"assigned value",
			"because:",
			"proven: xs[i] can be number or nil here",
			"claimed: n is declared as number",
			"missing proof: xs[i] is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here",
			"help: Guard `xs[i]` with a nil check",
		},
		RenderNotContains: []string{
			"want string",
			"^~",
		},
	})
}
