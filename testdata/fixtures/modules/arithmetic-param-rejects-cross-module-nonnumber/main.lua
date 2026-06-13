local provider = require("provider")

local CONFIG = { rate = 4 }

-- scale uses tokens as an arithmetic operand, which proves tokens must be a number.
-- The body forwards a cross-module record into that parameter, so the arithmetic
-- operand is a non-number value and the operation is rejected.
local function scale(tokens)
    return tokens * CONFIG.rate
end

local function run()
    local m = provider.meta()
    return scale(m) -- expect-error: expected number
end

return run
