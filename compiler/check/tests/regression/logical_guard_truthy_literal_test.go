package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// `A and "?" or v` evaluates v only when A is false, because a string literal
// is never falsy, so v is narrowed by the negation of A.
func TestLogicalGuard_TruthyLiteralNarrowsOrOperand(t *testing.T) {
	testutil.RunCases(t, []testutil.Case{
		{
			Name: "string literal",
			Code: `
local function label(v: number | string): string
	return type(v) ~= "string" and "?" or v
end
`,
			Stdlib: true,
		},
		{
			Name: "number literal",
			Code: `
local function count(v: number | string): number
	return type(v) ~= "number" and 0 or v
end
`,
			Stdlib: true,
		},
		{
			Name: "table literal",
			Code: `
local function rows(v: {string} | string): {string}
	return type(v) ~= "table" and {} or v
end
`,
			Stdlib: true,
		},
		{
			Name: "nil operand keeps the value unnarrowed",
			Code: `
local function label(v: number | string): string
	return type(v) ~= "string" and nil or v
end
`,
			WantError: true,
			Stdlib:    true,
		},
	})
}
