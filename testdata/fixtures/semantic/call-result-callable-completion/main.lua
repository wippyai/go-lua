type Event = {kind: string, payload: string?}
type Other = {n: number}

local function make()
    return function()
        local e: Event = {kind = "metric", payload = nil}
        return e
    end
end

-- The result term holds the complete callable a direct binding holds.
local produced: fun(): Event = make()

-- The same contract survives the write that binds it, so the call through the
-- bound term reaches the derived result.
local held = make()
local bound: fun(): Event = held
local through_bound: Event = held()
local inline: Event = (make())()

-- Parameters come from the callable's own surface, the result from its body.
local function make_taking()
    return function(n: number)
        local e: Event = {kind = "metric", payload = nil}
        return e
    end
end
local taking: fun(n: number): Event = make_taking()

-- A mismatched result refutes against the fused contract.
local wrong_result: fun(): Other = make() -- expect-error: cannot assign wrong_result

-- A mismatched parameter surface refutes too: the surface is the callable's
-- own, and completion never rewrites it.
local wrong_arity: fun(n: number): Event = make() -- expect-error: cannot assign wrong_arity
local wrong_parameter: fun(s: string): Event = make_taking() -- expect-error: cannot assign wrong_parameter

-- Fail-closed: a callable whose body derives no result keeps its bare surface,
-- so a declared result stays unproven.
local function make_opaque()
    return function(...)
        return ...
    end
end
local unfused: fun(): Event = make_opaque() -- expect-error: cannot assign unfused

-- A callee that declares its own result keeps that declaration as the sole
-- authority; completion never reaches it.
local function make_declared(): fun(): Event
    return function()
        local e: Event = {kind = "metric", payload = nil}
        return e
    end
end
local declared: fun(): Event = make_declared()

return "ok"
