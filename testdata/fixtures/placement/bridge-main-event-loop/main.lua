-- Distilled from bridge.lua:282-716 main event loop (kickside placement benchmark).
-- Per-site placement classes are documented in ../README.md.
type Inbound = { kind: string, id: string, body: {[string]: string} }
type Response = { ok: boolean, id: string }

-- Module-level cache: closure-captured actor state -> OwnedHeap.
local cache: {[string]: Inbound} = {}

local function main(batch: {Inbound}, sink: Channel<Inbound>): number
    local processed = 0                                   -- no Heap root
    for _, msg in ipairs(batch) do
        local key: string = msg.kind .. ":" .. msg.id     -- Stack: derived, dies each iteration
        local scratch: Response = { ok = true, id = msg.id } -- Stack: used locally then dropped
        cache[key] = msg                                  -- inbound parked in OwnedHeap cache
        processed = processed + 1
        if msg.kind == "forward" then
            sink:send(msg)                                -- forwarding path contributes SharedHeap demand
        end
        print(scratch.id)                                 -- scratch consumed locally, never escapes
    end
    return processed
end

return main
