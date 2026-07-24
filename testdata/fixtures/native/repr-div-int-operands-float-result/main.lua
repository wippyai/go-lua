-- `/` is float division in Lua: with both operands integer the result carrier
-- is still a float, licensed per binary instruction, never from operand arms.
local function ratio(a: integer, b: integer): number
    local q = a / b
    return q
end

return ratio
