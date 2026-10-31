package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestStrictLogicalFallbackNarrowing(t *testing.T) {
	cases := []struct {
		cluster string
		good    string
		bad     string
	}{
		{"N001", `local function f(r: any): (boolean, string?) return false, (r and tostring(r.error)) or "failed" end`, `local function f(r: any): (boolean, string?) return false, r.error end`},
		{"N002", `local function f(rows: any): number return (rows and rows[1] and tonumber(rows[1].n)) or 0 end`, `local function f(rows: any): number return rows[1].n end`},
		{"N003", `local function f(err: any): string return (err and tostring(err)) or "failed" end`, `local function f(err: any): string return err end`},
		{"N004", `local function f(flag: any): string return flag and "completed" or "error" end`, `local function f(flag: any): string return flag end`},
		{"N005", `local function f(flag: any): (nil, nil, string?) return nil, nil, flag and "ok" or nil end`, `local function f(flag: any): (nil, nil, string?) return nil, nil, flag end`},
		{"N007", `local function trim(v: any): string return "" end; local function f(v: any): string? return v and trim(v) or nil end`, `local function f(v: any): string? return v end`},
		{"N008", `local function f(v: any): string? return v and tostring(v) or nil end`, `local function f(v: any): string? return v end`},
		{"N009", `local function trim(v: any): string return "" end; local function f(v: any): string return v and trim(v) or "" end`, `local function f(v: any): string return v end`},
		{"N010", `local function g(v: string) end; local function f(v: any) g(v and "yes" or "no") end`, `local function g(v: string) end; local function f(v: any) g(v) end`},
		{"N011", `local function g(v: string?) end; local function f(v: any) g(v and "yes" or nil) end`, `local function g(v: string?) end; local function f(v: any) g(v) end`},
		{"N012", `local function g(a: any, b: any, c: any, d: any, e: string?) end; local function f(v: any) g(nil,nil,nil,nil,v and tostring(v) or nil) end`, `local function g(a: any, b: any, c: any, d: any, e: string?) end; local function f(v: any) g(nil,nil,nil,nil,v) end`},
		{"N013", `local function g(v: string?) end; local function f(v: any) g(v and "yes" or nil) end`, `local function g(v: string?) end; local function f(v: any) g(v) end`},
		{"N014", `local function g(v: string?) end; local function f(v: any) g(v and "yes" or nil) end`, `local function g(v: string?) end; local function f(v: any) g(v) end`},
		{"N015", `local function f(v: any): (nil, nil, string?) return nil, nil, v and tostring(v) or nil end`, `local function f(v: any): (nil, nil, string?) return nil, nil, v end`},
	}
	for _, tc := range cases {
		t.Run(tc.cluster, func(t *testing.T) {
			good := testutil.Check(tc.good, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
			if good.HasError() {
				t.Fatalf("narrowed expression rejected: %v", testutil.ErrorMessages(good.Diagnostics))
			}
			bad := testutil.Check(tc.bad, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
			if !bad.HasError() {
				t.Fatal("unnarrowed any accepted")
			}
		})
	}
}
