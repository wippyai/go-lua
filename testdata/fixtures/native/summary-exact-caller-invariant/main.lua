-- Contract: a monomorphic function whose result depends only on its parameters
-- has one exact, caller-invariant summary that serves every call site, which is
-- what licenses a single shared inline cache.

local function area(w: number, h: number): number
    return w * h
end

local a: number = area(2, 3)
local b: number = area(10, 20)

return a + b
