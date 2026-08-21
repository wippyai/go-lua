-- Distilled from enrichment_service.lua:240-346 main (kickside placement benchmark).
-- Debounce: the pending/last maps are actor-local; per-event derived values are frame-local/scalar.
type Event = { key: string, at: number, payload: string }

local pending: {[string]: Event} = {}   -- OwnedHeap: debounced events awaiting flush
local last: {[string]: number} = {}     -- OwnedHeap: last-seen timestamps

local function debounce(e: Event, window: number): boolean
    local prev: number? = last[e.key]                              -- Stack borrow
    local fresh: boolean = prev == nil or (e.at - prev) >= window  -- no Heap root
    if fresh then
        pending[e.key] = e             -- event retained -> OwnedHeap
        last[e.key] = e.at
    end
    return fresh
end

return debounce
