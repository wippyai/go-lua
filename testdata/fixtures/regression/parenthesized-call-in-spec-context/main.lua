local function pair(): (string, integer)
    return "a", 1
end

local function take(s: string, n: integer?): string
    return s .. tostring(n)
end

for _, v in ipairs({ (pair()) }) do
    local s: string = take(v)
    print(s)
end

local ok = take((pair()))
return { ok = ok }
