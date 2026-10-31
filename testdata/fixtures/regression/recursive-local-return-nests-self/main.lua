local function tree(n: integer)
    if n == 0 then
        return "leaf"
    end
    local t = {}
    t[1] = tree(n - 1)
    return t
end

local v = tree(2)
local s: string = v -- expect-error: cannot assign
if type(v) == "table" then
    local first = v[1]
    local leaf: string = first -- expect-error: cannot assign
end
