-- The same `or 0` fallback shape with annotated numeric operands.
-- The numeric carrier must survive the `or` logical selection into both operator operands.

local function add(a: number, b: number): number
    return (a or 0) + (b or 0)
end

return add
