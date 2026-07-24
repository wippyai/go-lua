-- The subject may be NaN, so the false edge of `<=` does not prove `>`. The branch row
-- must publish the defined comparison result without claiming a total ordering.

local function pick(a: number, b: number): number
    local x = a / b
    if x <= 1.0 then
        return x
    end
    return 1.0
end

return pick(0, 0)
