-- Contract: a generic function's summary is instantiation dependent; it must be
-- published as context sensitive and must not license a shared caller-invariant
-- inline cache.

local function id<T>(v: T): T
    return v
end

local n: number = id(42)
local s: string = id("hello")

return s .. tostring(n)
