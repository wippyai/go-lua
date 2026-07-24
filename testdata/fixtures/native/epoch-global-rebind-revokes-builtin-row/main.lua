-- A builtin_call fact for tostring is keyed on the binding, not the name: rebinding the
-- global revokes it, and the call after the rebind gets no builtin row.

local a = tostring(1)

tostring = function(v: any): string
    return "rebound"
end

local b = tostring(2)

return a .. b
