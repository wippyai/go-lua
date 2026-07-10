type Counts = { [string]: number }
local c: Counts = {}
c["a"] = 1
c["b"] = 2
local x: number? = c["a"]
if x ~= nil then
    return x
end
return 0
