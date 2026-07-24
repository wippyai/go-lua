-- Unary negation preserves the integer representation arm; Lua integer
-- arithmetic wraps, so the overflow disposition is closed, not promoting.
local function invert(n: integer): integer
    local m = -n
    return m
end

return invert
