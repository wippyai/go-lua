-- An operand whose declared type admits a table carrying __add cannot be proved to
-- stay out of metamethod dispatch, so the primitive dispatch class is withheld.

type Vec = { x: number }

local VecMT = {}
VecMT.__add = function(l: Vec, r: Vec): Vec
    return { x = math.max(l.x, r.x) }
end

local function combine(a: Vec, b: Vec): Vec
    return a + b
end

return combine(setmetatable({ x = 1 }, VecMT), setmetatable({ x = 2 }, VecMT))
