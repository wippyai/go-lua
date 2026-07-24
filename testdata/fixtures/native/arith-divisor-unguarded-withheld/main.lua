-- The same floor division without a guard. The divisor may be 0 or -1, so no divisor
-- property is published and neither integer-division trap can be dropped.

local function idiv(a: integer, b: integer)
    return a // b
end

return idiv
