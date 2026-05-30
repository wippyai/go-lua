package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZCapNarrowProbe reproduces the captured-optional narrowing patterns from
// the target fixtures in single files. Debug probe for the capture-seeding lane.
func TestZZCapNarrowProbe(t *testing.T) {
	cases := map[string]string{
		// event-bus-saga subscriber_builder.lua:116 shape: an optional captured into
		// a returned closure, guarded by `if decorator then decorator() end`.
		"closure-captured-optional-call": `
type Decorator = (string) -> string
local function build(decorator: Decorator?): (string) -> string
    return function(note: string): string
        if decorator then
            note = decorator(note)
        end
        return note
    end
end
return build
`,
		// service-locator locator.lua:26 shape: a module-captured optional, early
		// return guard, then return the now-non-nil value.
		"module-captured-early-return": `
type Services = { logger: string }
local M = {}
local _services: Services? = nil
function M.init(): Services
    local s: Services = { logger = "x" }
    _services = s
    return s
end
function M.get(): Services
    if not _services then
        return M.init()
    end
    return _services
end
return M
`,
		// SOUNDNESS: an UNGUARDED captured optional call must still error.
		"soundness-unguarded-captured-optional-call": `
type Decorator = (string) -> string
local function build(decorator: Decorator?): (string) -> string
    return function(note: string): string
        return decorator(note)
    end
end
return build
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			msgs := testutil.ErrorMessages(res.Diagnostics)
			if len(msgs) == 0 {
				t.Logf("NO DIAG")
			}
			for _, m := range msgs {
				t.Logf("DIAG: %s", m)
			}
		})
	}
}
