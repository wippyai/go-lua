local provider = require("provider")

local refuted = provider.finite(true)
local absent: number = refuted.duration

local partial = provider.finite(true)
local carried: number = partial.elapsed

local unenumerable = provider.opaque(true, function() return {kind = "x"} end)
local unknown: number = unenumerable.duration

return { absent, carried, unknown }
