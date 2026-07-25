-- A closure does not have to be passed as an argument to escape. Storing it in a
-- table and handing that table to an opaque sink makes it reachable, so the sink
-- may run it at the call or any time after. The captured field's flow narrowing
-- ends at the escape, not at a direct call of the closure.

type Config = { host: string? }
type Handlers = { fn: () -> () }

local function escaping(): string
    local cfg: Config = { host = "x" }
    local handlers: Handlers = {
        fn = function()
            cfg.host = nil
        end,
    }
    sink(handlers)
    local h: string = cfg.host -- expect-error
    return h
end

-- The same closure stays inside the function. Nothing reaches the container, so
-- no call site can run it and the field keeps the value the constructor wrote.
local function contained(): string
    local cfg: Config = { host = "x" }
    local handlers: Handlers = {
        fn = function()
            cfg.host = nil
        end,
    }
    local h: string = cfg.host
    return h .. tostring(handlers ~= nil)
end

return escaping, contained
