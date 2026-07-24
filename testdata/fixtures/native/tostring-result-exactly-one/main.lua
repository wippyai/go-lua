-- The tostring result is exactly one string: the open result transport into
-- the concat must not be published as an open tail.

local function label(n: integer): string
    return "n=" .. tostring(n)
end

return label(7)
