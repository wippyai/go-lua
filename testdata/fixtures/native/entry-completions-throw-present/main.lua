-- Contract: a body with a reachable error() call carries throw in its completion
-- set; the entry contract must not be published as normal-only.

local function checked(v: number): number
    if v < 0 then
        error("bad")
    end
    return v
end

return checked
