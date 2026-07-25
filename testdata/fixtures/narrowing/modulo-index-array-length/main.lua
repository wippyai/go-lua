-- Wrapping an integer index by the array's own length is in range by
-- construction: with #xs >= 1 proven, i % #xs lands in [0, #xs-1] whatever the
-- sign of i, so (i % #xs) + 1 lands in [1, #xs].
local function wrap_own_length(xs: {number}, i: integer): number
    if i >= 0 and #xs >= 1 then
        local v: number = xs[(i % #xs) + 1]
        return v
    end
    return 0
end

-- The wrap uses another array's length, which bounds nothing about zs: #ys can
-- exceed #zs, so the read stays optional.
local function wrap_other_length(zs: {number}, ys: {number}, i: integer): number
    if i >= 0 and #ys >= 1 and #zs >= 1 then
        local v: number = zs[(i % #ys) + 1] -- expect-error
        return v
    end
    return 0
end

-- A float dividend gives a float residue, so the wrapped index is not an array
-- position at all.
local function wrap_float_dividend(fs: {number}, x: number): number
    if x >= 0 and #fs >= 1 then
        local v: number = fs[(x % #fs) + 1] -- expect-error
        return v
    end
    return 0
end

return wrap_own_length, wrap_other_length, wrap_float_dividend
