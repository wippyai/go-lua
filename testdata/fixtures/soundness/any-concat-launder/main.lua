-- Concatenation does not validate an any operand. A __concat metamethod may
-- return any value at all, and an operand with no concat behavior raises, so
-- the result carries the operand's boundary rather than a proven string.

local function launder(x: any): string
    return "e: " .. x -- expect-error
end

-- Two proven strings produce a proven string.
local function proven(a: string, b: string): string
    return a .. b
end

-- tostring publishes a proven string for the same value.
local function stringified(x: any): string
    return "e: " .. tostring(x)
end

-- A concrete cast is runtime validation on the normal path.
local function casted(x: any): string
    return "e: " .. (x as string)
end

return launder, proven, stringified, casted
