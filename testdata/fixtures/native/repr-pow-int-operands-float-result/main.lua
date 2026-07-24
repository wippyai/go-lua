-- `^` is float exponentiation in Lua: two integer operands still produce a
-- float result carrier.
local function power(a: integer, b: integer): number
    local p = a ^ b
    return p
end

return power
