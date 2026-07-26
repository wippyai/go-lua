-- A loop-carried counter's lower bound is a property of the body's write set,
-- not of any value the fixed point holds: widening collapses the counter to its
-- primitive, so a bound read off one trip's value states nothing about the
-- others. The bound this body can state is that the counter never falls below
-- the constant it entered the cycle with, and that holds only when every write
-- inside the cycle is the counter itself plus a non-negative constant.
--
-- Representation decides whether the arithmetic keeps the bound. Lua's integer
-- addition is closed and wraps at the maximum integer, so an integer counter's
-- increments prove nothing about its value however many of them run. A number
-- counter is not closed that way and keeps the seed.

-- The bound holds: seed 1, one increment by a constant, number representation.
-- With the upper bound the loop condition states, the read is in range.
local function scan(xs: {number}): number
    local i: number = 1
    local total: number = 0
    while i <= #xs do
        local v: number = xs[i]
        total = total + v
        i = i + 1
    end
    return total
end

-- A decrement is not a non-negative step. The counter can fall below its seed,
-- so the loop condition alone leaves the read unbounded from below.
local function descending(xs: {number}): number
    local i: number = 1
    while i <= #xs do
        local v: number = xs[i] -- expect-error
        i = i - 1
    end
    return 0
end

-- A step this body cannot name as a constant carries no sign. An arbitrary
-- number parameter may be negative, so the counter is not proven monotone.
local function stepped(xs: {number}, step: number): number
    local i: number = 1
    while i <= #xs do
        local v: number = xs[i] -- expect-error
        i = i + step
    end
    return 0
end

-- The increments are non-negative and constant, but the representation wraps:
-- at the maximum integer the next increment lands at the minimum, so the seed
-- is not a bound on the counter's value.
local function wrapping(xs: {number}): number
    local i: integer = 1
    while i <= #xs do
        local v: number = xs[i] -- expect-error
        i = i + 1
    end
    return 0
end

-- A zero step is non-negative, so the seed survives it.
local function stationary(xs: {number}, more: () -> boolean): number
    local i: number = 1
    while i <= #xs do
        local v: number = xs[i]
        i = i + 0
    end
    return 0
end

-- A second assignment from outside the cycle is part of the same write set: a
-- seed below one states no bound this operand may carry.
local function reseeded(xs: {number}, low: boolean): number
    local i: number = 1
    if low then
        i = 0
    end
    while i <= #xs do
        local v: number = xs[i] -- expect-error
        i = i + 1
    end
    return 0
end

return scan, descending, stepped, wrapping, stationary, reseeded
