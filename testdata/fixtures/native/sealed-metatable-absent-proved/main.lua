-- The literal never reaches setmetatable, so its metatable is proven absent and
-- the __index chain walk at the read site is elided.

local counters = { hits = 0, misses = 0 }

local function total(): number
    return counters.hits + counters.misses
end

return total()
