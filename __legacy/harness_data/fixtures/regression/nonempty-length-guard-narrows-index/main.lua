-- A non-empty guard proves at least one element, so x[1] and x[#x] read non-optional.
local function first(xs: {number}): number
    if #xs > 0 then
        local a: number = xs[1]
        local b: number = xs[#xs]
        return a + b
    end
    return 0
end

-- #xs ~= 0 is the same non-empty proof.
local function head(xs: {number}): number
    if #xs ~= 0 then
        local a: number = xs[1]
        return a
    end
    return 0
end

return first, head
