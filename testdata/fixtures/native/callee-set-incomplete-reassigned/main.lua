-- Contract: a local binding that carries two distinct function values on merging
-- paths has an Incomplete callee set of cardinality 2; the row must not claim a
-- singleton.

local function a(x: number): number
    return x + 1
end

local function b(x: number): number
    return x - 1
end

local function dispatch(flag: boolean): number
    local f = a
    if flag then
        f = b
    end
    return f(1)
end

return dispatch
