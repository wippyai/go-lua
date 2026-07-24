-- `/` yields a VM float for every operand arm in Lua, so `r` is provably a
-- float representation at the multiplication site.
local function scaled_ratio(a: number, b: number): number
    local r = a / b
    return r * 2.0
end

return scaled_ratio
