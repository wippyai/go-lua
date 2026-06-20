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

return arith, transitive, cross, no_lower, reassigned, mutated
