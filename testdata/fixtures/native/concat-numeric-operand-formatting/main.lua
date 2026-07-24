-- The numeric operand is formatted by Lua integer semantics at the concat
-- site; the conversion is not left to the backend.

local function key(i: integer): string
    return "k" .. i
end

return key(3)
