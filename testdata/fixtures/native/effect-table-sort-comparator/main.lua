-- EFFECT ROW: table.sort transfers control into a caller-supplied comparator
-- that can raise. The site's row must publish error presence and the callback
-- control transfer, which makes the site a safepoint.
local function by_length(a: string, b: string): boolean
    if #a == #b then
        error("unordered")
    end
    return #a < #b
end

local names: {string} = { "gamma", "al", "beta" }
table.sort(names, by_length)

return names
