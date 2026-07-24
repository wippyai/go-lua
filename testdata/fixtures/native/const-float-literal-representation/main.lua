-- `42.0` is a float machine word; the constant must not be normalised to the
-- integer 42 on the strength of its value alone.
local function offset(base: number): number
    local n = 42.0
    return base + n
end

return offset
