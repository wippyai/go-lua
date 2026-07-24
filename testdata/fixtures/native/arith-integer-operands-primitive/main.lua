-- Integer operands make the machine arm singular for the operator itself.
-- The overflow disposition must ride with the row: an integer add can leave the
-- integer arm, so the JIT still needs to know which arm the result lands on.

local function add(a: integer, b: integer)
    return a + b
end

return add
