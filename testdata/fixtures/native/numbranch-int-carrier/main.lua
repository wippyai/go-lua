-- The same branch syntax with an integer subject. The published carrier must be the
-- integer representation, and integer comparison is a total order.

local function f(x: integer): integer
    if x < 0 then
        return 0
    end
    return x
end

return f
