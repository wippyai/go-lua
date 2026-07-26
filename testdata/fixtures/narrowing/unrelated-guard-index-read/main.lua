-- A guard that states nothing about the index leaves the read soundly-optional.
-- The arm is reachable for every array the declaration admits, so the
-- annotation that admits no nil is refuted inside it exactly as it is outside.
local function unrelated_length_guard(xs: {number}, i: number): number
    if #xs >= 3 then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- A guard on the index alone raises its floor without bounding it above.
local function index_floor_only(xs: {number}, i: number): number
    if i >= 1 then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- A guard of a family the numeric domain states nothing about is still a
-- reachable arm, so it withholds no obligation either.
local function unrelated_literal_guard(xs: {number}, i: number): number
    if i ~= 0 then
        local v: number = xs[i] -- expect-error
        return v
    end
    return 0
end

-- Both bounds together discharge the obligation: the index is in range.
local function discharged(xs: {number}, i: number): number
    if i >= 1 and i <= #xs then
        local v: number = xs[i]
        return v
    end
    return 0
end

return unrelated_length_guard, index_floor_only, unrelated_literal_guard, discharged
