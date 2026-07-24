-- Contract: a function that returns a forwarded call's whole result list has an
-- open result tail; the entry contract publishes prefix 0 with an open tail and
-- must never let a spread result be read as exactly one value.

local function inner(...: number)
    return ...
end

local function fwd(...: number)
    return inner(...)
end

return fwd(1, 2, 3)
