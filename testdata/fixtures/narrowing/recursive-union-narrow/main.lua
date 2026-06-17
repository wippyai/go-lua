type Json = number | string | { [string]: Json }

local function depth(v: Json): number
    if type(v) == "number" then
        return 0
    end
    if type(v) == "string" then
        return 0
    end
    return 1
end

local x: Json = { a = 1, b = { c = "deep" } }
return depth(x)
