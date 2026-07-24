-- Contract: a variadic function publishes its vararg policy as part of the entry
-- layout: a fixed parameter prefix followed by an open, caller-sized tail.

local function sum(...: number): number
    local total: number = 0
    for _, v in ipairs({...}) do
        total = total + v
    end
    return total
end

return sum(1, 2, 3)
