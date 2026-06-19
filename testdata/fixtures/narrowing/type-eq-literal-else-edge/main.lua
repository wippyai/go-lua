local function need_string(s: string): string
    return s
end

-- On the else edge of type(v) == "number", v excludes number and is string.
local v: number | string = 5
if type(v) == "number" then
    return 0
else
    return #need_string(v)
end
