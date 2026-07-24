-- PUBLICATION MONOTONICITY: adding a use, or a possible target, must never
-- STRENGTHEN a grant. base.lua already uses M.scale once; this module adds a
-- second, indirected use, so the callee set here must be weaker or equal.
local base = require("base")

local dispatch: (number) -> number = base.scale

local second: number = dispatch(2)

return base.first + second
