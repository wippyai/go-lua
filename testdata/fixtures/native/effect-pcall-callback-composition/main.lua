-- EFFECT ROW: pcall's row is the composition of its callback's row. A fully
-- analyzable local callback must compose into the site, not degrade it to open.
local function halve(n: number): number
    return n / 2
end

local ok, value = pcall(halve, 8)

return ok, value
