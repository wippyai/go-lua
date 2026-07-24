-- The empty string is truthy in Lua: a length test is not a truth test, so
-- the condition is statically decided.
local function empty_is_truthy(): integer
    local s = ""
    if s then
        return 1
    else
        return 0
    end
end

return empty_is_truthy
