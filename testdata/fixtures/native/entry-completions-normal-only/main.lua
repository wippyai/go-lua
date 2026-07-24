-- Contract: a pure arithmetic body completes only normally; the entry contract
-- must publish the whole completion vocabulary as known with only normal
-- present, otherwise the row is not a fixed signature.

local function area(w: number, h: number): number
    return w * h
end

return area(3, 4)
