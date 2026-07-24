-- One activation begins at entry and one closure captures it: the row must
-- state a unique, completely covered epoch root.

local function counter(): () -> integer
    local n: integer = 0
    return function(): integer
        n = n + 1
        return n
    end
end

return counter()
