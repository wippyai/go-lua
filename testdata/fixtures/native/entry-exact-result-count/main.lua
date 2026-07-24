-- Contract: a function whose every return carries the same fixed arity publishes
-- an exact result count in its entry contract.

local function split(v: number): (number, number)
    return v, v + 1
end

local lo, hi = split(4)

return lo + hi
