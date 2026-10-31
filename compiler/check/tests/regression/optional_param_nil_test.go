package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestOptionalParametersAcceptExplicitNil(t *testing.T) {
	result := testutil.Check(`
local m: string? = nil
assert(true, nil)
assert(true, m)
string.rep("a", 2, nil)
local n: number? = 42
assert(true, n)
`, testutil.WithStdlib())
	messages := testutil.ErrorMessages(result.Diagnostics)
	if len(messages) != 1 || !strings.Contains(messages[0], "argument 2:") || !strings.Contains(messages[0], "got number?") {
		t.Fatalf("expected only the incompatible non-nil message to fail, got %v", messages)
	}
}

func TestOptionalParametersAcceptExpandedNil(t *testing.T) {
	result := testutil.Check(`
local function pair(): (boolean, string?) return true, nil end
assert(pair())
local function bad(): (boolean, number?) return true, 42 end
assert(bad())
`, testutil.WithStdlib())
	messages := testutil.ErrorMessages(result.Diagnostics)
	if len(messages) != 1 || !strings.Contains(messages[0], "argument 2:") || !strings.Contains(messages[0], "got number?") {
		t.Fatalf("expected only the incompatible non-nil message to fail, got %v", messages)
	}
}

func TestHostAritySensitiveParameters(t *testing.T) {
	for _, tt := range []struct {
		code string
		want string
	}{
		{`math.random(nil)`, "argument 1:"},
		{`math.random(1, nil)`, "argument 2:"},
		{`table.remove({1}, nil)`, "argument 2:"},
		{`table.sort({1}, nil)`, "argument 2:"},
		{`table.insert({}, "wrong", "value")`, "argument 2:"},
		{`string.byte("abc", nil)`, ""},
		{`string.byte("abc", 2, nil)`, ""},
		{`table.concat({"a"}, nil, nil, nil)`, ""},
		{`local n: number? = table.remove({1}); local m: number? = table.remove({1}, 1)`, ""},
	} {
		t.Run(tt.code, func(t *testing.T) {
			messages := testutil.ErrorMessages(testutil.Check(tt.code, testutil.WithStdlib()).Diagnostics)
			if tt.want == "" {
				if len(messages) != 0 {
					t.Fatalf("unexpected errors: %v", messages)
				}
			} else if len(messages) != 1 || !strings.Contains(messages[0], tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, messages)
			}
		})
	}
}
