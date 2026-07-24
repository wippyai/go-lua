-- A metatable_seal fact established at setmetatable is revoked when the installed __index
-- table later gains a member: a snapshot taken at install time is not a seal.
type Counter = { n: number, get: (self: Counter) -> number }

local Methods = {}
Methods.__index = Methods

function Methods.get(self: Counter): number
    return self.n
end

local c: Counter = { n = 1, get = Methods.get }
setmetatable(c, Methods)
local first = c:get()

function Methods.bump(self: Counter): number
    return self.n + 1
end

return first + c:get()
