-- Contract: calling a value imported through an any-typed path yields an Unknown
-- callee set; cross-module blindness must never be reported as Complete.

local make = require("helper")

local v = make()

return v
