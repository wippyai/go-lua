package lua

import (
	"strings"
	"testing"
)

// A protected call that fails after its coroutine was suspended returns
// exactly what the same call returns when it never suspends.
func TestSuspendedProtectedErrorMatchesSynchronous(t *testing.T) {
	calls := map[string]string{
		"pcall":          `pcall(function() %s error("boom") end)`,
		"xpcall":         `xpcall(function() %s error("boom") end, function(e) return "H:" .. e end)`,
		"xpcall_failing": `xpcall(function() %s error("boom") end, function(e) error("handler failed", 0) end)`,
	}
	shapes := map[string]string{
		"multret":     `return select("#", 0, %s)`,
		"one_result":  `local a = %s return tostring(a)`,
		"two_results": `local a, b = %s return tostring(a) .. ":" .. tostring(b)`,
		"padded":      `local a, b, c = %s return tostring(a) .. ":" .. tostring(b) .. ":" .. tostring(c)`,
	}
	for cname, call := range calls {
		for sname, shape := range shapes {
			t.Run(cname+"/"+sname, func(t *testing.T) {
				L := NewState()
				defer L.Close()
				run := func(body string) string {
					src := strings.Replace(shape, "%s", strings.Replace(call, "%s", body, 1), 1)
					ret, _, _ := runToCompletion(t, L, src, -1)
					if len(ret) != 1 {
						t.Fatalf("unexpected results %v", ret)
					}
					return ret[0].String()
				}
				sync, suspended := run(""), run("coroutine.yield(1)")
				if sync != suspended {
					t.Fatalf("synchronous %q, suspended %q", sync, suspended)
				}
			})
		}
	}
}
