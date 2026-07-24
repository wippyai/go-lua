-- A `number | string` operand may hold a non-numeric string. The numeric-string
-- coercion disposition is unresolved, so the primitive dispatch class is withheld
-- rather than silently dropped.

local function total(a: number | string, b: number): number
    return a + b
end

return total
