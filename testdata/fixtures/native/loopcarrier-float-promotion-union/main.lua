-- One arm feeds a float into the accumulator. The published carrier is the promoted
-- arm and must never be narrowed back to the integer representation, or the merge at
-- the backedge would be a lie.

local function total(n: integer): number
    local sum = 0
    for i = 1, n do
        if i % 2 == 0 then
            sum = sum + i
        else
            local half = i / 2
            sum = sum + half
        end
    end
    return sum
end

return total
