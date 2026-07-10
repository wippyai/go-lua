-- An index with no proven bound stays soundly-optional: xs[i] for an
-- unconstrained i may be nil, so it is not a non-optional element.
local function pick(xs: {number}, i: number): number
    return xs[i] -- expect-error
end

-- An index one past the proven length is out of range and stays optional.
local function over(xs: {number}): number
    local v: number = xs[#xs + 1] -- expect-error
    return v
end

return pick, over
