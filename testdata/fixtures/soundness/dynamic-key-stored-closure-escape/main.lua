-- A closure stored under a key the analysis cannot resolve is still held by the
-- container. Handing that container to an opaque sink makes the closure
-- reachable, so the sink may run it at the call or any time after and the
-- captured field's flow narrowing ends at the escape. The unnamed slot is what
-- makes this distinct from soundness/stored-closure-later-invocation: no member
-- coordinate names the callable, so the container's own inventory is the only
-- record of it.

type Config = { host: string? }

local function escaping(): string
    local key: string = tostring(1)
    local cfg: Config = { host = "x" }
    local handlers = {}
    handlers[key] = function()
        cfg.host = nil
    end
    sink(handlers)
    local h: string = cfg.host -- expect-error
    return h
end

-- The same unnamed slot holds the same closure, but nothing reaches the
-- container. No call site can run it, so the field keeps the value the
-- constructor wrote.
local function contained(): string
    local key: string = tostring(1)
    local cfg: Config = { host = "x" }
    local handlers = {}
    handlers[key] = function()
        cfg.host = nil
    end
    local h: string = cfg.host
    return h .. tostring(handlers ~= nil)
end

return escaping, contained
