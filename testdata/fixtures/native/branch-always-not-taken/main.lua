-- The mirror partition: a constant-decided condition that is never true, so
-- the then arm is the dead instruction stream.
local function guard(): integer
    local cap = 3
    if cap > 5 then
        return 0
    else
        return cap
    end
end

return guard
