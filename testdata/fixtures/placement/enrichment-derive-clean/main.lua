-- CLEAN positive, distilled from enrichment_service.lua:90-105.
-- Derive-then-drop for DB rows: the rows die local; only a scalar string escapes.
type Row = { id: string, name: string, score: number }

local function summarize(rows: {Row}): string
    local best: string = ""              -- Stack
    local top: number = 0                -- no Heap root
    for _, r in ipairs(rows) do
        if r.score > top then
            top = r.score
            best = r.name                -- derived scalar, stays local
        end
    end
    return best                          -- rows dropped; a string leaves the frame
end

return summarize
