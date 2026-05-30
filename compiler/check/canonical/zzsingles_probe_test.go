package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestZZSinglesProbe(t *testing.T) {
	cases := map[string]string{
		"callback-nested": `
local function outer(f: (number) -> number)
    return f(10)
end
local result: number = outer(function(x: number): number
    return x * 2
end)
return result
`,
		"service-locator-earlyret": `
type Services = { name: string }
local _services: Services? = nil
local function init(): Services
    local s = { name = "x" }
    _services = s
    return s
end
local function get(): Services
    if not _services then
        return init()
    end
    return _services
end
return get
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			t.Log("--- CANON ---")
			res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			for _, m := range testutil.ErrorMessages(res.Diagnostics) {
				t.Logf("CANON: %s", m)
			}
			t.Log("--- LEGACY ---")
			res2 := testutil.Check(src, testutil.WithStdlib())
			for _, m := range testutil.ErrorMessages(res2.Diagnostics) {
				t.Logf("LEGACY: %s", m)
			}
		})
	}
}
