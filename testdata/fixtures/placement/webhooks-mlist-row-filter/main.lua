-- Distilled from webhooks/repo.lua:236-265 M.list (kickside placement benchmark).
-- Runtime row-filter: the new returned container has an OwnedHeap return demand;
-- selected source routes remain input-dependent containment evidence.
type Row = { id: string, url: string, active: boolean }

local function list(rows: {Row}, only_active: boolean): {Row}
    local out: {Row} = {}                        -- returned container -> OwnedHeap demand
    for _, r in ipairs(rows) do
        if (not only_active) or r.active then
            out[#out + 1] = r                    -- runtime-selected containment route
        end
    end
    return out
end

return list
