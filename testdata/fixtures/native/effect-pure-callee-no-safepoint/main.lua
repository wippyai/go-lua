-- EFFECT ROW: a callee proven allocation-absent, yield-absent and error-absent
-- is not a safepoint, so the loop's preemption poll may be elided. The row is
-- exhaustive: it is closed over the whole module generation.
local function blend(a: number, b: number): number
    return a * 2 + b * 3
end

local total: number = 0
local n: number = 0
while n < 16 do
    total = blend(total, n)
    n = n + 1
end

return total
