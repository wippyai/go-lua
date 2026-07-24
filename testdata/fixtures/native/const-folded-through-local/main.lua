-- A constant survives an intermediate binding: the fold reaches the use, it
-- does not stop at the first local.
local function total(): integer
    local n = 10
    local m = n + 5
    return m
end

return total
