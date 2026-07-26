-- A guard whose subject is a body local is decided by the declaration exactly
-- as one over the formal is: the local holds only what this body derived from
-- its own seeds and literals. The arm is reachable and states no lower bound on
-- the index, so the element read stays optional.
local function from_zero(xs: {string}): string
    local i: number = 0
    if i <= #xs then
        local v: string = xs[i] -- expect-error
        return v
    end
    return ""
end

local function from_negative(xs: {string}): string
    local i: number = -5
    if i <= #xs then
        local v: string = xs[i] -- expect-error
        return v
    end
    return ""
end

-- A local carrying a formal's value narrows exactly as the formal does: the
-- floor alone leaves the read unbounded above.
local function local_alias(xs: {string}, i: number): string
    local j = i
    if j >= 1 then
        local v: string = xs[j] -- expect-error
        return v
    end
    return ""
end

-- A store the analysis reaches can empty the slot the sequence border rests on,
-- so every proof measured against that border stops holding at the store.
local function store_revokes_border(xs: {string}, i: number): string
    if #xs >= 1 then
        local n = #xs
        xs[i] = nil
        local v: string = xs[n] -- expect-error
        return v
    end
    return ""
end

local function store_revokes_in_range(xs: {string}, i: number): string
    if i >= 1 and i <= #xs then
        local k = 1
        xs[k] = nil
        local v: string = xs[i] -- expect-error
        return v
    end
    return ""
end

-- The border read itself is sound under a proven non-empty length: the border
-- the operator returns is positive, so it names a written slot.
local function border_under_floor(xs: {string}): string
    if #xs >= 1 then
        local n = #xs
        local v: string = xs[n]
        return v
    end
    return ""
end

-- Without that floor the empty border remains available and xs[0] is nil.
local function border_without_floor(xs: {string}): string
    local n = #xs
    local v: string = xs[n] -- expect-error
    return v
end

-- Both index bounds discharge the read; the constant ceiling does the same when
-- the proven floor already covers it.
local function bounded_by_length(xs: {string}, i: number): string
    if i >= 1 and i <= #xs then
        local v: string = xs[i]
        return v
    end
    return ""
end

local function bounded_by_constant(xs: {string}, i: number): string
    if #xs >= 3 and i >= 1 and i <= 3 then
        local v: string = xs[i]
        return v
    end
    return ""
end

return from_zero, from_negative, local_alias, store_revokes_border, store_revokes_in_range,
    border_under_floor, border_without_floor, bounded_by_length, bounded_by_constant
