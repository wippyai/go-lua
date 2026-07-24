-- SEND SAFETY: a payload table holding a closure carries that closure's captured
-- environment across the boundary. The verdict is REFUTED, never "unknown".
local pid: string = "collector"

local secret: string = "s-1"

process.send(pid, "handler", {
    run = function(): string
        return secret
    end,
})

return pid
