-- CLEAN positive. A table allocated in a callee and returned to its caller
-- never leaves the actor, so both issued placement-return-escape rows are
-- proven actor-local retention.
local function make()
    local value = { value = 1 }
    return value
end

local escaped = make()

return true
