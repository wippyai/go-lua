-- A `number | string` subject admits a lexicographic string comparison and a mixed
-- comparison that throws, so no numeric branch carrier can be published on either edge.

local function f(x: number | string): number
    if x < 0 then
        return 0
    end
    return 1
end

return f
