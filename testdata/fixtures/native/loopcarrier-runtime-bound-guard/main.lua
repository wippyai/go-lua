-- The trip count is a runtime value with a proven range. That licenses ONE sealed
-- preheader guard over `n` protecting the accumulator, not a per-iteration overflow
-- test on every backedge.

local function total(n: integer): integer
    if n < 0 or n > 1000 then
        return 0
    end
    local sum = 0
    for i = 1, n do
        sum = sum + i
    end
    return sum
end

return total
