local ssub = string.sub

local function read(s: string, pos: integer)
    if pos == 0 then return true end
    return ssub(s, pos)
end

local v = read("abc", 1)
if type(v) ~= "boolean" then
    local text: string = v
end
