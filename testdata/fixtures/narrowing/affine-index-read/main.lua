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

-- The identity survives an ordinary binding: k names the same shifted term the
-- guard bounded, so the read through it is the read through i + 1.
local function stored(xs: {string}, i: integer): string
    if i >= 1 and i + 1 <= #xs then
        local k: integer = i + 1
        local v: string = xs[k]
        return v
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

-- The base is replaced after the guard, so the relation is stated about a
-- number the body has left and the shifted term inherits nothing from it.
local function rebased(xs: {string}, i: integer, j: integer): string
    if i >= 1 and i + 1 <= #xs then
        i = j
        local v: string = xs[i + 1] -- expect-error
        return v
    end
    return ""
end

-- The array is mutated after the guard, so its length may have moved and the
-- shifted slot is no longer proven.
local function mutated(xs: {string}, i: integer): string
    if i >= 1 and i + 1 <= #xs then
        xs[#xs] = nil
        local v: string = xs[i + 1] -- expect-error
        return v
    end
    return ""
end

-- The shift is computed before the base is replaced, so the stored term and the
-- guard's relation describe two different numbers.
local function stored_then_rebased(xs: {string}, i: integer, j: integer): string
    if i >= 1 and i + 1 <= #xs then
        local k: integer = i + 1
        i = j
        local v: string = xs[k] -- expect-error
        return v
    end
    return ""
end

return forward, backward, window, stored, past_bound, underflow, rebased, mutated,
    stored_then_rebased
