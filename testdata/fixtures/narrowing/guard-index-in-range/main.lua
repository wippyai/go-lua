-- A positive floor plus an upper-bound guard proves xs[i] in-range non-optional.
local function at(xs: {number}, i: number): number
    if i >= 1 and i <= #xs then
        local v: number = xs[i]
        return v
    end
    return 0
end

-- The strict upper bound i < #xs proves i <= #xs-1, also in range.
local function at_strict(xs: {number}, i: number): number
    if i >= 1 and i < #xs then
        local v: number = xs[i]
        return v
    end
    return 0
end

-- Operand order does not matter: 0 < i and #xs >= i prove the same bounds.
local function at_flipped(xs: {number}, i: number): number
    if 0 < i and #xs >= i then
        local v: number = xs[i]
        return v
    end
    return 0
end

-- Nested guards prove the same as a conjunction.
local function at_nested(xs: {number}, i: number): number
    if i >= 1 then
        if i <= #xs then
            local v: number = xs[i]
            return v
        end
    end
    return 0
end

-- No upper bound: i may exceed the length, so the read stays optional.
local function no_upper(xs: {number}, i: number): number
    if i >= 1 then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- No positive floor: i may be zero or negative, so the read stays optional.
local function no_lower(xs: {number}, i: number): number
    if i <= #xs then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- The guard bounds i by a different array's length, proving nothing about xs.
local function wrong_array(xs: {number}, ys: {number}, i: number): number
    if i >= 1 and i <= #ys then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- A write that can change the array length invalidates the in-range proof.
local function mutated(xs: {number}, i: number): number
    if i >= 1 and i <= #xs then
        xs[#xs] = nil
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

return at, at_strict, at_flipped, at_nested, no_upper, no_lower, wrong_array, mutated
