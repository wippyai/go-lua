-- A call in the final argument position expands at the position its own
-- argument occupies, and its leading result is the value that lands there. That
-- position is exact whatever the rest of the expansion turns out to be, so the
-- parameter contract at it is decided from the same value an ordinary argument
-- would carry. What the spread adds past the positions the list names has no
-- proven count, so nothing there is decided and no arity is claimed.

local function count(): number
    return 1
end

local function pair(): (number, number)
    return 1, 2
end

local function need(s: string): number
    return 1
end

local function need_two(a: string, b: string): number
    return 1
end

local function need_mixed(a: number, b: string): number
    return 1
end

-- The leading result lands at the parameter the spread's own position states.
local function spread_head(): number
    return need(count()) -- expect-error: argument 1 is 1, not string
end

-- Binding the same result first reaches the identical contract.
local function bound_head(): number
    local value = count()
    return need(value) -- expect-error: argument 1 (value) is 1, not string
end

-- Arguments ahead of the spread keep their own exact positions.
local function spread_tail_position(): number
    return need_two("head", count()) -- expect-error: argument 2 is 1, not string
end

-- A satisfied contract stays clean: the leading result is what the parameter
-- states, and the expansion beyond it is not evidence against the call.
local function spread_satisfied(): number
    return need_mixed(count())
end

-- The positions a multi-result expansion fills past the ones this list names
-- are supplied by results the call binds no term for. Parameter b states string
-- and receives a number at runtime, and that stays undecided here rather than
-- being asserted from an arity the spread does not prove.
local function spread_beyond_named(): number
    return need_mixed(pair())
end

-- A spread may expand to no values at all, so an argument list longer than the
-- parameter list proves no count. Nothing is claimed about the extra position.
local function spread_arity_unclaimed(): number
    return need_mixed(1, "text", count())
end

return spread_head, bound_head, spread_tail_position, spread_satisfied, spread_beyond_named, spread_arity_unclaimed
