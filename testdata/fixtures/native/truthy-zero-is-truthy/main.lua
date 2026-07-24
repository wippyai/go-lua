-- Only nil and false are falsy in Lua, so `0` is truthy: the condition is
-- statically decided and the else arm is dead.
local function zero_is_truthy(): string
    local n = 0
    if n then
        return "yes"
    else
        return "no"
    end
end

return zero_is_truthy
