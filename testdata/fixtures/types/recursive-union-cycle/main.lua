type Json = number | string | boolean | {Json} | { [string]: Json }

local function size(v: Json): number
    return 1
end
local doc: Json = { a = 1, b = "x", c = { true, 2, { nested = "deep" } } }
return size(doc)
