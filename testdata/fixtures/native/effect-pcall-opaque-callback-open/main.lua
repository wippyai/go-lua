-- EFFECT ROW (OPEN): a callback arriving through a parameter has no known row,
-- so the pcall site's row must be published OPEN. Absence of an observed effect
-- is not evidence of its absence.
local function guard(fn: () -> number): boolean
    local ok = pcall(fn)
    return ok
end

return guard
