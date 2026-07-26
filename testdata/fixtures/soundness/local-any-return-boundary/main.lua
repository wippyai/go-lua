-- A local callee that declares any at a return slot publishes that boundary to
-- its caller. The declaration is the contract the body was checked against, so
-- the body's own concrete outcome never stands in for the claim: every position
-- that requires a concrete type refutes until a runtime validation discharges it.

local function decode(): any
    return "text"
end

local function need(s: string): number
    return 1
end

-- A typed local requires a proof the boundary cannot supply.
local function into_local(): number
    local s: string = decode() -- expect-error: is any, not string
    return 1
end

-- A typed parameter requires the same proof.
local function into_argument(): number
    local raw = decode()
    return need(raw) -- expect-error: argument 1 (raw) is any, not string
end

-- The result reaches that parameter unbound as well: a call in the final
-- argument position expands at the position it occupies, and its leading result
-- is the value that lands there.
local function into_spread_argument(): number
    return need(decode()) -- expect-error: argument 1 is any, not string
end

-- A declared return type is refuted by the boundary it was handed.
local function into_return(): string
    return decode() -- expect-error: comes from any/unknown
end

-- A record field states its own contract, so the field carries the refutation.
local function into_field(): {message: string}
    return { message = decode() } -- expect-error: message
end

-- Concatenation does not validate the boundary its operand crossed.
local function into_concat(): string
    local raw = decode()
    return "e: " .. raw -- expect-error: comes from any/unknown
end

-- The operand carries that boundary in its own value, so an unbound result
-- taints the concatenation exactly as the bound one does.
local function into_spread_concat(): string
    return "e: " .. decode() -- expect-error: comes from any/unknown
end

-- A runtime type test is the boundary's own validator.
local function validated(): string
    local raw = decode()
    if type(raw) == "string" then
        return raw
    end
    return ""
end

-- A member-stored closure and a method declare the same boundary.
local carrier = {}

function carrier.decode(): any
    return "text"
end

function carrier:read(): any
    return "text"
end

local function into_member(): number
    local s: string = carrier.decode() -- expect-error: is any, not string
    local m: string = carrier:read() -- expect-error: is any, not string
    return 1
end

-- A declared callable states the boundary at its own binding, so the literal
-- bound to it is checked against that declaration rather than read through it.
local declared: fun(): any = function()
    return "text"
end

local function into_declared(): number
    local s: string = declared() -- expect-error: is any, not string
    return 1
end

-- A declared surface states the same boundary at the members it holds, whether
-- a member is applied directly or reached as a method.
type Surface = {
    handler: fun(): any,
    read: fun(self: Surface): any,
}

local function into_surface_field(surface: Surface): string
    return surface.handler() -- expect-error: comes from any/unknown
end

local function into_surface_method(surface: Surface): string
    return surface:read() -- expect-error: comes from any/unknown
end

local surface: Surface = {
    handler = function(): any
        return "text"
    end,
    read = function(self: Surface): any
        return "text"
    end,
}

-- Only the declared any slot carries the boundary. Slot 1 states string and
-- keeps the concrete contract it declared.
local function pair(): (string, any)
    return "text", "text"
end

local function into_pair(): number
    local head, tail = pair()
    local ok: string = head
    local bad: string = tail -- expect-error: is any, not string
    return 1
end

-- A result the body alone derives is not a claim. This callee declares no
-- return and its body's result is unresolved, so the slot stays top: the
-- consumer owes an ordinary unproven claim, never an any boundary.
local function inferred(handler: fun())
    return handler()
end

local function into_inferred(): number
    local s: string = inferred(function() end) -- expect-error: is not proven
    return 1
end

return into_local, into_argument, into_spread_argument, into_return, into_field, into_concat, into_spread_concat, validated, into_member, into_declared,
    into_surface_field(surface), into_surface_method(surface), into_pair, into_inferred
