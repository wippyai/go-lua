-- Every table is truthy in Lua, so a non-optional table condition is a test
-- the JIT eliminates outright.
type Box = { v: integer }

local function unwrap(b: Box): integer
    if b then
        return b.v
    else
        return 0
    end
end

return unwrap
