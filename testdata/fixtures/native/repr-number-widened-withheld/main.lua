-- A `number` local assigned an integer on one path and a float on another has
-- no statically singular representation arm; the widened type grants nothing
-- at the use site.
local function pick(flag: boolean): number
    local v: number = 0
    if flag then
        v = 1
    else
        v = 0.5
    end
    return v + v
end

return pick
