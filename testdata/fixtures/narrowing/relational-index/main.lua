-- Index arithmetic: i + 1 <= #xs proves xs[i+1] in range.
local function arith(xs: {number}, i: number): number
    if i >= 1 and i + 1 <= #xs then
        local v: number = xs[i + 1]
        return v
    end
    return 0
end

-- Transitive ordering: i < j and j <= #xs prove i < #xs.
local function transitive(xs: {number}, i: number, j: number): number
    if i >= 1 and i < j and j <= #xs then
        local v: number = xs[i]
        return v
    end
    return 0
end

-- Cross-variable length equality: #a == #b carries i <= #a to b[i].
local function cross(a: {number}, b: {number}, i: number): number
    if #a == #b and i >= 1 and i <= #a then
        local v: number = b[i]
        return v
    end
    return 0
end

-- Soundness: no positive-index proof, so i + 1 could be <= 0.
local function no_lower(xs: {number}, i: number): number
    if i + 1 <= #xs then
        local v: number = xs[i + 1] -- expect-error
        return v
    end
    return 0
end

-- Soundness: the index is reassigned after the relational guard.
local function reassigned(xs: {number}, i: number, j: number): number
    if i >= 1 and i < j and j <= #xs then
        i = j
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- Soundness: the array is mutated after the guard, so its length may change.
local function mutated(a: {number}, b: {number}, i: number): number
    if #a == #b and i >= 1 and i <= #a then
        b[#b] = nil
        local v: number = b[i] -- expect-error
        return v
    end
    return 0
end

-- Sum bounded by a variable length: i + j <= #xs with j >= 0 proves i <= #xs.
-- Beyond difference logic; the linear certificate backend discharges it.
local function sum(xs: {number}, i: number, j: number): number
    if i >= 1 and j >= 0 and i + j <= #xs then
        local v: number = xs[i]
        return v
    end
    return 0
end

-- Sum proves the other positive operand too: j <= i + j <= #xs with i >= 0.
local function sum_other(xs: {number}, i: number, j: number): number
    if i >= 0 and j >= 1 and i + j <= #xs then
        local v: number = xs[j]
        return v
    end
    return 0
end

-- Soundness: without j >= 0, i + j <= #xs does not bound i, since j may be
-- negative. The read must stay optional.
local function sum_no_floor(xs: {number}, i: number, j: number): number
    if i >= 1 and i + j <= #xs then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- Soundness: the index is reassigned past the bound after the sum guard.
local function sum_reassigned(xs: {number}, i: number, j: number): number
    if i >= 1 and j >= 0 and i + j <= #xs then
        i = i + 100
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- Soundness: the array is mutated after the sum guard, so its length may change.
local function sum_mutated(xs: {number}, i: number, j: number): number
    if i >= 1 and j >= 0 and i + j <= #xs then
        xs[#xs] = nil
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- Soundness: the sum is on the lower side, so #xs <= i + j gives no upper bound
-- on i.
local function sum_wrong_side(xs: {number}, i: number, j: number): number
    if i >= 1 and j >= 0 and #xs <= i + j then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- Soundness: reading the other operand needs i >= 0; without it j may exceed #xs
-- when i is negative.
local function sum_other_no_floor(xs: {number}, i: number, j: number): number
    if j >= 1 and i + j <= #xs then
        local v: number = xs[j] -- expect-error
        return v
    end
    return 0
end

return arith, transitive, cross, no_lower, reassigned, mutated, sum, sum_other,
    sum_no_floor, sum_reassigned, sum_mutated, sum_wrong_side, sum_other_no_floor
