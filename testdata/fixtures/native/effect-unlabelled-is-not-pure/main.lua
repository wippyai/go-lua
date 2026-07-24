-- EFFECT ROW: a stdlib entry that carries no effect label is not thereby pure.
-- The missing label is a withholding condition, never a grant.
local function stamp()
    return os.clock()
end

local at = stamp()

return at
