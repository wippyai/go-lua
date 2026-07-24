-- A vararg tail makes the element count dynamic, so the exact bounded-capacity
-- row is withheld rather than rounded up from the written prefix.
local function pack(a: number, ...: number): {number}
    local rows = {a, ...}
    return rows
end

return pack
