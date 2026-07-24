-- setmetatable is called on the table: metatable-absent is withheld and the
-- __index chain walk cannot be elided.

local defaults = { hits = 0 }
local counters = { misses = 0 }
setmetatable(counters, { __index = defaults })

return counters.misses
