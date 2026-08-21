-- Distilled from channel/hub.lua:44-226 main (kickside placement benchmark).
-- Inbound slices are parked in actor state -> shared-handle retain (zero-copy).
type Msg = { topic: string, seq: number, data: {[string]: string} }

local parked: {[string]: Msg} = {}   -- OwnedHeap: retained inbound registry
local order: {string} = {}           -- OwnedHeap: insertion index

local function hub(batch: {Msg})
    for _, m in ipairs(batch) do
        parked[m.topic] = m          -- inbound handle retained in the owned registry
        order[#order + 1] = m.topic  -- derived string appended into the owned index
    end
end

return hub
