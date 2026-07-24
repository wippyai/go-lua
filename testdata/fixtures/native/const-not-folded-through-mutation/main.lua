-- A local written on a conditional path holds no constant at the merge; two
-- opaque arms cannot be folded into a single machine word.
local function pick(flag: boolean, lo: integer, hi: integer): integer
    local n = lo
    if flag then
        n = hi
    end
    return n * n
end

return pick
