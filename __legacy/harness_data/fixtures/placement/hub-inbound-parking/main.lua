-- Distilled from channel/hub.lua:44-226 main (kickside placement benchmark).
-- Inbound slices are parked in actor state -> shared-handle retain (zero-copy).
type Msg = { topic: string, seq: number, data: {[string]: string} }

local parked: {[string]: Msg} = {}   -- ActorLocal: retained inbound registry
local order: {string} = {}           -- ActorLocal: insertion index

local function hub(batch: {Msg})
    for _, m in ipairs(batch) do
        parked[m.topic] = m          -- inbound handle retained -> shared-handle retain (zero-copy)
        order[#order + 1] = m.topic  -- derived string appended into the actor-local index
    end
end

return hub
