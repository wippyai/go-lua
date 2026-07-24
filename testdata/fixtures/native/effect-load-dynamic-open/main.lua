-- EFFECT ROW (OPEN): `load` on a runtime string admits new code into the world.
-- The row must be published OPEN with module loading present; it can never be
-- closed, because the loaded body is not in the analysis unit.
local function compile(src: string)
    local chunk = load(src)
    return chunk
end

return compile
