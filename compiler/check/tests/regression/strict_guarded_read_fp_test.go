package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestStrictRepeatedGuardedRead(t *testing.T) {
	cases := []struct{ cluster, good, bad string }{
		{"N006", `local function f(ids: any): string? if type(ids) == "table" and type(ids[1]) == "string" and ids[1] ~= "" then return ids[1] end return nil end`, `local function f(ids: any): string? return ids[1] end`},
		{"N016", `local function f(msg: any): string if type((msg :: any).caption) == "string" then return (msg :: any).caption end return "" end`, `local function f(msg: any): string return (msg :: any).caption end`},
	}
	for _, tc := range cases {
		t.Run(tc.cluster, func(t *testing.T) {
			good := testutil.Check(tc.good, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
			if good.HasError() {
				t.Fatalf("guarded read rejected: %v", testutil.ErrorMessages(good.Diagnostics))
			}
			bad := testutil.Check(tc.bad, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
			if !bad.HasError() {
				t.Fatal("unguarded read accepted")
			}
		})
	}
}
