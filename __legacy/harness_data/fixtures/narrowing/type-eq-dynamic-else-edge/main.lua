local function need_string(s: string): string
    return s
end

local kind: "number" = "number"
local v: number | string = 5
if type(v) == kind then
    return 0
else
    return #need_string(v)
end
