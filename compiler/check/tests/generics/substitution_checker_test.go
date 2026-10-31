package generics

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestGenericSubstitutionChecker(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"shadowed nested function", `
			local function outer<T>(x: T): T
				local function inner<T>(y: T): T return y end
				local s: string = inner("word")
				return x
			end
			local n: integer = outer(42)
		`},
		{"generic alias", `
			type Box<T> = { value: T }
			local b: Box<string> = { value = "word" }
			local s: string = b.value
		`},
		{"function typed argument", `
			local function apply<T, U>(x: T, f: (T) -> U): U return f(x) end
			local function length(s: string): integer return #s end
			local n: integer = apply("word", length)
		`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib())
			if result.HasError() {
				t.Fatalf("checker errors: %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
