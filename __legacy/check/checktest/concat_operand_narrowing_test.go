package checktest

import "testing"

// A concat operand guarded present must not be reported as possibly nil, whether
// its optional came from a declaration, a stdlib call (string.match), or a local
// function - the flow-solved value state proves presence in every case. This is
// the inferred-optional analogue of the declared-optional narrowing.
func TestCheckConcatOperandNarrowsInferredOptional(t *testing.T) {
	clean := map[string]string{
		"stdlib-match-guard": `
local function f(s: string): string
    local r = s:match("^%s*(.-)%s*$")
    if not r or r == "" then return "" end
    return "url/" .. r
end
return f`,
		"local-fn-optional-guard": `
local function get(b: boolean): string?
    if b then return "x" end
    return nil
end
local function f(b: boolean): string
    local r = get(b)
    if not r then return "" end
    return "url/" .. r
end
return f`,
		"truthy-positive-branch": `
local function get(b: boolean): string?
    if b then return "x" end
    return nil
end
local function f(b: boolean): string
    local r = get(b)
    if r then return "url/" .. r end
    return ""
end
return f`,
	}
	for name, src := range clean {
		if d := Check(src, WithStdlib()).Diagnostics; len(d) != 0 {
			t.Errorf("%s: diagnostics = %#v, want concat operand proven present", name, d)
		}
	}
}

// An unguarded optional concat operand is still reported - the fix narrows on
// flow evidence, it does not suppress real nil risk.
func TestCheckConcatOperandUnguardedOptionalStillReported(t *testing.T) {
	flagged := map[string]string{
		"declared-optional": `
local function f(s: string?): string
    return "url/" .. s
end
return f`,
		"stdlib-match-unguarded": `
local function f(s: string): string
    local r = s:match("p")
    return "url/" .. r
end
return f`,
	}
	for name, src := range flagged {
		if d := Check(src, WithStdlib()).Diagnostics; len(d) == 0 {
			t.Errorf("%s: want a may-be-nil diagnostic for the unguarded optional operand", name)
		}
	}
}
