-- A `false` constant condition is always falsy: the then arm is dead, and the
-- class must say which arm rather than report a bare boolean.
local function never_true(): integer
    local b = false
    if b then
        return 1
    else
        return 0
    end
end

return never_true
