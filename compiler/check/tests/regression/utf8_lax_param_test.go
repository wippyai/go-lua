package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Lua 5.4 utf8.len, utf8.codepoint and utf8.codes take an optional lax flag.
func TestUtf8LaxParameter(t *testing.T) {
	for _, code := range []string{
		`local n, p = utf8.len("abc", 1, -1, true)`,
		`local c = utf8.codepoint("abc", 1, 1, true)`,
		`for p, c in utf8.codes("abc", true) do end`,
	} {
		t.Run(code, func(t *testing.T) {
			messages := testutil.ErrorMessages(testutil.Check(code, testutil.WithStdlib()).Diagnostics)
			if len(messages) != 0 {
				t.Fatalf("unexpected errors: %v", messages)
			}
		})
	}
}
