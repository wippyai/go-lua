-- A parameter map with concrete number values is likewise invariant: accepting
-- a wider alias would let a string write corrupt a later number-typed read.
local function f(narrow: {[string]: number}): number
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
