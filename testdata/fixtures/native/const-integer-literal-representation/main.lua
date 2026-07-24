-- A constant fact must carry its machine representation: `42` and `42.0` are
-- different machine words, so the value alone is not a licence.
local function offset(base: integer): integer
    local n = 42
    return base + n
end

return offset
