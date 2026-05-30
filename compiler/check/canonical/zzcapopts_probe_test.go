package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZCapOptsProbe reproduces the captured-options under-inference from the
// providers-open-retry-captured-options fixtures in a single file. A module-level
// local is initialized to nil, written inside a closure (captured_options = opts),
// then read after a not_nil guard. The capture write must teach the captured
// variable a table type so the post-guard read does not index nil/unknown.
func TestZZCapOptsProbe(t *testing.T) {
	cases := map[string]string{
		// Minimal: a module local set to nil, written via an upvalue inside a
		// closure that is then invoked, then read after a not_nil-style guard.
		"captured-write-then-read": `
local captured = nil

local function setter(opts)
    captured = opts
end

setter({ retry = { max_attempts = 3, initial_delay = 100 } })

if captured == nil then error("nil") end
if captured.retry == nil then error("nil retry") end

local attempts: number = captured.retry.max_attempts
local delay: number = captured.retry.initial_delay
return attempts, delay
`,
		// Closure stored in a table field (as in the fixtures' with_options), then
		// invoked through a method-style call.
		"captured-write-via-table-method": `
local captured = nil

local function make()
    return {
        with_options = function(self, opts)
            captured = opts
            return self
        end,
    }
end

local chain = make()
chain:with_options({ retry = { max_attempts = 3, initial_delay = 100 } })

if captured == nil then error("nil") end
if captured.retry == nil then error("nil retry") end

local attempts: number = captured.retry.max_attempts
return attempts
`,
		// The fixture shape: the writing closure is a table field invoked indirectly
		// through a SEPARATE function (open), not called directly at top level.
		"captured-write-via-indirect-open": `
local captured = nil

local function get_contract()
    return {
        with_options = function(self, opts)
            captured = opts
            return self
        end,
    }
end

local function open(overrides)
    local c = get_contract()
    c = c:with_options({ retry = overrides.retry })
    return c
end

open({ retry = { max_attempts = 3, initial_delay = 100 } })

if captured == nil then error("nil") end
if captured.retry == nil then error("nil retry") end

local attempts: number = captured.retry.max_attempts
return attempts
`,
		// Exact fixture guard shape: not_nil is a function (returns its arg or
		// errors) but its return is DISCARDED; narrowing must come from the
		// captured variable's own type, and the write must teach it a table type.
		"captured-write-notnil-func-guard": `
local function not_nil(val, msg)
    if val == nil then error(msg) end
    return val
end

local captured = nil

local function get_contract()
    return {
        with_options = function(self, opts)
            captured = opts
            return self
        end,
    }
end

local function open(overrides)
    local c = get_contract()
    c = c:with_options({ retry = overrides.retry })
    return c
end

open({ retry = { max_attempts = 3, initial_delay = 100 } })

not_nil(captured, "captured expected")
not_nil(captured.retry, "retry expected")

local attempts: number = captured.retry.max_attempts
local delay: number = captured.retry.initial_delay
return attempts, delay
`,
		// The writing closure is DEFINED but the engine cannot see it invoked at
		// top level (invocation is hidden behind an opaque/cross-module call). This
		// mirrors the fixture: does the closure-body write still teach captured a
		// table type for the post-guard read?
		"captured-write-closure-not-invoked-here": `
local captured = nil

local function get_contract()
    return {
        with_options = function(self, opts)
            captured = opts
            return self
        end,
    }
end

-- get_contract returns the table but with_options is never called in this scope
local c = get_contract()

if captured == nil then error("nil") end
if captured.retry == nil then error("nil retry") end

local attempts: number = captured.retry.max_attempts
return attempts
`,
		// Exact fixture nesting: the writing closure is nested 3 deep inside table
		// constructors assigned to a table field (providers._contract). The
		// closure-body write must still teach captured a table type.
		"captured-write-deep-nested-field-assign": `
local providers = {}

local captured = nil

providers._contract = {
    get = function(_id)
        return {
            with_options = function(self, opts)
                captured = opts
                return self
            end,
        }, nil
    end,
}

if captured == nil then error("nil") end
if captured.retry == nil then error("nil retry") end

local attempts: number = captured.retry.max_attempts
return attempts
`,
		// Type probe: after the closure write, captured should be table? (nil-union),
		// so assigning it to a number annotation errors with "nil-or-table", not
		// plain "nil". If it errors plain "nil", the write-back was not applied.
		"typeprobe-captured-after-write": `
local captured = nil
local function setter(opts) captured = opts end
setter({ retry = 1 })
local probe: number = captured
return probe
`,
		// Does a not_nil-style function (errors on nil, return discarded) narrow an
		// already-optional value? This is the guard half of the fixture, decoupled
		// from the capture-typing half. If clean, the only remaining gap is the
		// capture type being plain nil instead of nil|table.
		"optional-narrowed-by-notnil-func-discarded-return": `
type Retry = { max_attempts: number, initial_delay: number }
type Opts = { retry: Retry }
local function not_nil(val, msg)
    if val == nil then error(msg) end
    return val
end
local captured: Opts? = nil
not_nil(captured, "expected")
not_nil(captured.retry, "retry expected")
local attempts: number = captured.retry.max_attempts
return attempts
`,
		// If the captured-write source is gradual any, the joined capture type is
		// any? -- after the not_nil guard the read is gradual-clean.
		"optional-any-narrowed-by-notnil-func": `
local function not_nil(val, msg)
    if val == nil then error(msg) end
    return val
end
local captured: any = nil
not_nil(captured, "expected")
not_nil(captured.retry, "retry expected")
local attempts: number = captured.retry.max_attempts
return attempts
`,
		// Type probe for the indirect-open single-file case: what does captured
		// resolve to after the closure write reached via a local indirect call?
		"typeprobe-indirect-open": `
local captured = nil
local function get_contract()
    return {
        with_options = function(self, opts)
            captured = opts
            return self
        end,
    }
end
local function open(overrides)
    local c = get_contract()
    c = c:with_options({ retry = overrides.retry })
    return c
end
open({ retry = { max_attempts = 3, initial_delay = 100 } })
local probe: number = captured
return probe
`,
		// SOUNDNESS: capture never written -> stays nil -> read must still error.
		"soundness-captured-never-written": `
local captured = nil

local function setter(opts)
    -- never assigns captured
    return opts
end

setter({ retry = 1 })

local attempts: number = captured.retry
return attempts
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
