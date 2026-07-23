local function make_counter(): () -> number
    local n = 0
    return function(): number
        n = n + 1
        return n
    end
end
local c = make_counter()
return c() + c()
