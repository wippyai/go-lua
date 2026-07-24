-- Contract: a local function binding that is never reassigned, stored or passed
-- proves a Complete callee set of cardinality 1 at every call site.

local function scale(x: number): number
    return x * 2
end

local a: number = scale(3)
local b: number = scale(a)

return a + b
