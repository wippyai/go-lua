-- A bounded for-loop induction variable indexes within the proven length, so the
-- element read drops its soundly-optional nil and is usable as the element type.
local function sum(xs: {number}): number
    local total: number = 0
    for i = 1, #xs do
        local v: number = xs[i]
        total = total + v
    end
    return total
end

-- ipairs gives the same in-range guarantee for the value.
local function sum_pairs(xs: {number}): number
    local total: number = 0
    for _, v in ipairs(xs) do
        local n: number = v
        total = total + n
    end
    return total
end

-- A while loop whose condition bounds the index by #xs proves the read in-range.
local function first(xs: {number}): number
    local i: number = 1
    while i <= #xs do
        local v: number = xs[i]
        if v > 0 then
            return v
        end
        i = i + 1
    end
    return 0
end

return sum, sum_pairs, first
