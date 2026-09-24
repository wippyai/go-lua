local r = require("r_sdk")

local function eval_ok(code: string): any
    local env = r.eval(code)
    local _ = env.error
    return env.result
end

return { eval_ok = eval_ok }
