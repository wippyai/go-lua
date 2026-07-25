-- A populated map cannot be widened covariantly: the wider alias could write a
-- string into a slot that the source map and compiled read trust as number.
local function f(): number
    local narrow: {[string]: number} = {}
    narrow["k"] = 1
    local wide: {[string]: number | string} = narrow -- expect-error
    wide["k"] = "boom"
    local v = narrow["k"]
    if v then
        local n: number = v
        return n
    end
    return 0
end

return f
