-- A positive floor plus an upper bound proves the index present, so xs[i]
-- reads T rather than T?.
local function at(xs: {number}, i: number): number
    if i >= 1 and i <= #xs then
        local v: number = xs[i]
        return v
    end
    return 0
end

return at
