-- Integer arithmetic is closed: adding two integers yields an integer, and an
-- overflow wraps inside the integer range instead of promoting to a float. An
-- unbounded increment loop therefore keeps the carrier's representation, and what
-- it leaves unproven is the carrier's VALUE. Representation drifts only where an
-- arm produces a float, and the loop's own wraparound is exactly what forbids
-- treating "starts at 1 and only grows" as a lower bound.

-- Every arm increments by an integer, so the representation survives the loop and
-- the integer annotation holds however many iterations run.
local function preserved_representation(start: integer, more: () -> boolean): integer
    local i: integer = start
    while more() do
        i = i + 1
    end
    local j: integer = i
    return j
end

-- The counter is monotone in the arithmetic, not in the value: at the maximum
-- integer the increment wraps to the minimum, so the initial floor of 1 does not
-- survive and an upper-bound guard alone cannot put the counter in range.
local function wrapped_floor(xs: {string}, more: () -> boolean): string
    local i: integer = 1
    while more() do
        i = i + 1
    end
    if i <= #xs then
        local v: string = xs[i] -- expect-error
        return v
    end
    return ""
end

-- Nothing bounds the counter from above either, so a floor alone leaves the read
-- past the end of the array.
local function unbounded_ceiling(xs: {string}, more: () -> boolean): string
    local i: integer = 1
    while more() do
        i = i + 1
    end
    if i >= 1 then
        local v: string = xs[i] -- expect-error
        return v
    end
    return ""
end

-- Division is the drift: it yields a float on every operand pair, so the join at
-- the loop head carries number and number is not an integer.
local function float_arm(more: () -> boolean, halve: () -> boolean): integer
    local i: integer = 7
    while more() do
        if halve() then
            i = i / 2
        else
            i = i + 1
        end
    end
    local j: integer = i -- expect-error
    return j
end

return preserved_representation, wrapped_floor, unbounded_ceiling, float_arm
