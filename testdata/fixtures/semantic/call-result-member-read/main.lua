type Box = {v: number}

-- A returned table literal: the sealed shape is the result's own authority.
local function build()
    return {v = 1, tag = "unit"}
end
local held = build()
local bound_v: number = held.v
local inline_v: number = (build()).v
local inline_tag: string = (build()).tag

-- A returned local the body never annotated keeps the literal shape, and the
-- read off the result term sees exactly what the read off a bound cell sees.
local function unannotated()
    local e = {kind = "metric"}
    return e
end
local bound_kind: number = (unannotated()).kind -- expect-error: cannot assign unannotated(...).kind

-- A returned formal is the caller's own table, read through the result term.
local function identity(b: Box)
    return b
end
local formal_v: number = (identity({v = 1})).v

-- A closed shape proves a slot it does not name absent, so a read of that slot
-- refutes rather than falling open.
local missing_v: number = (build()).absent -- expect-error: cannot assign build(...).absent

-- Fail-closed: a container the callee left open at a key it could not resolve
-- states nothing about the slot this read selects.
local function opaque(key: string)
    local out = {}
    out[key] = 1
    return out
end
local opaque_v = (opaque("k")).v

return "ok"
