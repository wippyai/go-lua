-- Nothing connects the result arm of one iteration to the input arm of the next unless
-- the carrier's entry representation and every transition edge, including the backedge,
-- are published together. Without that the loop pays a tag test every iteration.

local function total(n: integer): integer
    local sum = 0
    for i = 1, n do
        sum = sum + i
    end
    return sum
end

return total
