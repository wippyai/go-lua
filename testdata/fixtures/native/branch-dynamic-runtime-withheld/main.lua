-- A condition on a runtime parameter has no static partition: neither arm may
-- be declared dead and neither test may be dropped.
local function guard(cap: integer): integer
    if cap > 5 then
        return cap
    else
        return 0
    end
end

return guard
