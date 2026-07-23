-- Distilled from bridge.lua:282-716 main event loop (kickside placement benchmark).
-- Per-site placement classes are documented in ../README.md.
type Inbound = { kind: string, id: string, body: {[string]: string} }
type Response = { ok: boolean, id: string }

-- Module-level cache: closure-captured actor state -> ActorLocal.
local cache: {[string]: Inbound} = {}

local function main(batch: {Inbound}, sink: Channel<Inbound>): number
    local processed = 0                                   -- Scalar
    for _, msg in ipairs(batch) do
        local key: string = msg.kind .. ":" .. msg.id     -- FrameLocal: derived, dies each iteration
        local scratch: Response = { ok = true, id = msg.id } -- FrameLocal: used locally then dropped
        cache[key] = msg                                  -- inbound parked in ActorLocal cache (shared-handle retain)
        processed = processed + 1
        if msg.kind == "forward" then
            sink:send(msg)                                -- input-dependent forward -> deferred+promote
        end
        print(scratch.id)                                 -- scratch consumed locally, never escapes
    end
    return processed
end

return main
