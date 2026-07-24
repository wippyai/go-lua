-- false is not nil: the claim assert throws only on the nil arm and passes
-- false through unchanged.

local x: boolean | nil = false
return x!
