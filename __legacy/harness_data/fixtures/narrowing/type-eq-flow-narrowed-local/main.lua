local function need_number(n: number): number
    return n
end

local function pick(): string
    return "number"
end

-- k is a plain local of declared type string; the equality guard flow-narrows
-- it to the literal "number", which type(v) == k then reads to narrow v.
local k: string = pick()
local v: number | string = 5
if k == "number" then
    if type(v) == k then
        need_number(v)
    end
end
