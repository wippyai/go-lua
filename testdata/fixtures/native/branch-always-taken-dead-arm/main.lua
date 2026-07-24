-- A condition decided by constants partitions statically: the else arm is an
-- instruction stream the JIT never emits.
local function limit(): integer
    local cap = 10
    if cap > 5 then
        return cap
    else
        return 0
    end
end

return limit
