-- The typed producer row stands on its own: the neighbouring cast neither
-- supplies it nor licenses it, and it survives the cast being refused.

local x: any = 42
local y: number = x :: number -- expect-error: is not proven
return y
