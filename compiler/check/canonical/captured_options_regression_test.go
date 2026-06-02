package canonical_test

import "testing"

func TestCapturedOptionsRegression(t *testing.T) {
	cases := map[string]string{
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
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalClean(t, src)
		})
	}
}

func TestCapturedOptionsNeverWrittenStaysNil(t *testing.T) {
	src := `
local captured = nil

local function setter(opts)
    -- never assigns captured
    return opts
end

setter({ retry = 1 })

local attempts: number = captured.retry
return attempts
`
	requireCanonicalDiagnosticContains(t, src, "cannot index type nil")
}

func TestCapturedOptionsDirectWritePrecisionGap(t *testing.T) {
	src := `
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
`
	requireCanonicalClean(t, src)
}

func TestCapturedOptionsAnyNarrowingRequiresScalarProof(t *testing.T) {
	src := `
local function not_nil(val, msg)
    if val == nil then error(msg) end
    return val
end
local captured: any = nil
not_nil(captured, "expected")
not_nil(captured.retry, "retry expected")
local attempts: number = captured.retry.max_attempts
return attempts
`
	requireCanonicalDiagnosticContains(t, src, "cannot assign any to number")
}

func TestCapturedOptionsAnyFieldTypeGuardFeedsConcreteBoundary(t *testing.T) {
	src := `
local captured: any = nil
if type(captured.retry.max_attempts) == "number" then
    local attempts: number = captured.retry.max_attempts
    return attempts
end
return 0
`
	requireCanonicalClean(t, src)
}

func TestCapturedOptionsIndirectOpenFeedsCapturedCellWithoutGuard(t *testing.T) {
	src := `
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
local attempts: number = captured.retry.max_attempts
local delay: number = captured.retry.initial_delay
return attempts, delay
`
	requireCanonicalClean(t, src)
}
