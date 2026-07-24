-- An `integer`-annotated parameter is provably a VM integer at the operand
-- site: the representation arm is exact, not a widened `number`.
local function step(n: integer): integer
    local next_value = n + 1
    return next_value
end

return step
