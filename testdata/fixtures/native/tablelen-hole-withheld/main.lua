-- A nil hole inside the filled prefix makes the sequence border non-unique;
-- the raw length fact is withheld rather than read off the array header.
local function fill(n: number): number
    local t: {number} = {}
    for i = 1, n do
        t[i] = i
    end
    t[5] = nil
    return #t
end

return fill(10)
