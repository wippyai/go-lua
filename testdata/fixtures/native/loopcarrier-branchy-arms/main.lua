-- Three distinct `sum` recurrences under three arms of one loop body. The carrier row
-- must cover every arm and every backedge, not one representative arm.

local function classify(n: integer): integer
    local sum = 0
    for i = 1, n do
        if i % 3 == 0 then
            sum = sum + i
        elseif i % 2 == 0 then
            sum = sum - i
        else
            sum = sum + 1
        end
    end
    return sum
end

return classify
