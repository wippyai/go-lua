local function need_number(n: number): number
    return n
end

local v: number | string = 5
if type(v) == "number" then
    return 0
else
    return need_number(v)
end
