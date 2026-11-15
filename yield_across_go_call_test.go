package lua

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A yield inside Lua code that a Go function runs synchronously cannot
// suspend that Go function. It raises an error at the yield, with or without
// preemption.
func TestYieldAcrossGoCallRaises(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"gsub_callback", `
local ok, e = pcall(string.gsub, 'abc', '.', function(c) coroutine.yield(c) end)
return ok, e`},
		{"sort_comparator", `
local ok, e = pcall(table.sort, {3, 2, 1}, function(a, b) coroutine.yield(1) return a < b end)
return ok, e`},
		{"gsub_callback_unprotected_inside", `
return pcall(string.gsub, 'abc', '.', function(c)
  local ok, e = pcall(coroutine.yield, c)
  return tostring(ok) .. ':' .. tostring(e)
end)`},
	}
	for _, tc := range cases {
		for _, budget := range []int64{-1, 1, 3} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, budget), func(t *testing.T) {
				out, _, ok := diffRun(tc.src, nil, fixedBudget(budget), true, 1000, time.Time{})
				if !ok {
					t.Fatal("no progress")
				}
				if out.err != "" {
					t.Fatalf("unexpected error %q", out.err)
				}
				if strings.Contains(out.log, "yield:") {
					t.Fatalf("yield reached the host:\n%s", out.log)
				}
				if !strings.Contains(out.rets, "attempt to yield across a C-call boundary") {
					t.Fatalf("budget %d: results %s lack the boundary error", budget, out.rets)
				}
			})
		}
	}
}

// A coroutine resumed from inside a Go callback yields to that callback.
func TestYieldWithinCoroutineInsideGoCall(t *testing.T) {
	const src = `
local s = string.gsub('abc', '.', function(c)
  local co = coroutine.wrap(function() coroutine.yield(1) return 2 end)
  return tostring(co() + co())
end)
local y = coroutine.yield('after')
return s, y`
	for _, budget := range []int64{-1, 1, 3} {
		out, _, ok := diffRun(src, nil, fixedBudget(budget), true, 1000, time.Time{})
		if !ok || out.err != "" {
			t.Fatalf("budget %d: %v %q", budget, ok, out.err)
		}
		want := "\"333\" | \"after\""
		if out.rets != want || !strings.Contains(out.log, `yield: "after"`) {
			t.Fatalf("budget %d: rets %s log %s", budget, out.rets, out.log)
		}
	}
}
