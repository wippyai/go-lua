-- The Wippy calculator_add shape with no annotations.
-- An `or 0` fallback is not numeric execution authority: the truthy arm can still be a
-- numeric string, a non-number, or a table carrying __add, so dispatch is withheld.

local function add(a, b)
    return (a or 0) + (b or 0)
end

return add
