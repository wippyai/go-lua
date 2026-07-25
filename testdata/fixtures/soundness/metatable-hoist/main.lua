-- A metatable-backed __index read can run arbitrary code on every access, so
-- config.limit inside this loop is never hoistable.
type Config = { limit: number }

local ticks = 0
local config = setmetatable({ limit = 0 }, {
    __index = function(_self, _key): number
        ticks = ticks + 1
        return ticks
    end,
})
rawset(config, "limit", nil)
local total = 0
local i = 0
while i < 3 do
    total = total + config.limit
    i = i + 1
end
return total
