local function need_string(s: string): string
    return s
end

local x: number = 5
local kind: string = "number"
if type(x) == kind then
    need_string(42)
end
