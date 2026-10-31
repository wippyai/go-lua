-- The audit's W8 repro (wippy-dataflow/metatable-class-methods): the value
-- setmetatable returns carries the methods of its metatable's __index, also
-- when it is returned through a constructor.
type Reader = { n: number, get: (self: Reader) -> number }

local methods = {}
function methods.get(self: Reader): number return self.n end
local mt = { __index = methods }

local function new(): Reader
    local o = { n = 1 }
    return setmetatable(o, mt)
end

local r = new()
local v: number = r:get()
return { new = new, v = v }
