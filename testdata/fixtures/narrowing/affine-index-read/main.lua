-- An affine index term is in range when the term's own bounds are proven, not
-- when the base variable's are. The carrier is integer so the term lands on a
-- slot: a fractional index misses every array element whatever its bounds.

-- The upper bound is stated on the term: i + 1 <= #xs with i >= 1 puts i + 1 in
-- [2, #xs].
local function forward(xs: {string}, i: integer): string
    if i >= 1 and i + 1 <= #xs then
        local v: string = xs[i + 1]
        return v
    end
    return ""
end

-- A negative offset takes its floor from the base: i >= 2 puts i - 1 at or above
-- 1, and i <= #xs keeps it below the length.
local function backward(xs: {string}, i: integer): string
    if i >= 2 and i <= #xs then
        local v: string = xs[i - 1]
        return v
    end
    return ""
end

-- One guard bounds both offsets: i >= 2 floors i - 1 and i + 1 <= #xs ceilings
-- i + 1, so the whole window is in range.
local function window(xs: {string}, i: integer): string
    if i >= 2 and i + 1 <= #xs then
        local before: string = xs[i - 1]
        local after: string = xs[i + 1]
        return before .. after
    end
    return ""
end

-- i + 1 <= #xs leaves i + 2 one slot past the end, so the wider offset stays
-- optional.
local function past_bound(xs: {string}, i: integer): string
    if i >= 1 and i + 1 <= #xs then
        local v: string = xs[i + 2] -- expect-error
        return v
    end
    return ""
end

-- i >= 1 floors i, not i - 1: at i == 1 the term is 0 and xs[0] is nil.
local function underflow(xs: {string}, i: integer): string
    if i >= 1 and i <= #xs then
        local v: string = xs[i - 1] -- expect-error
        return v
    end
    return ""
end

return forward, backward, window, past_bound, underflow
