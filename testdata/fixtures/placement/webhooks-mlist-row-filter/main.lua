-- Distilled from webhooks/repo.lua:236-265 M.list (kickside placement benchmark).
-- Runtime row-filter: membership of the returned subset is input-dependent -> deferred+promote.
type Row = { id: string, url: string, active: boolean }

local function list(rows: {Row}, only_active: boolean): {Row}
    local out: {Row} = {}                        -- returned container escapes to caller
    for _, r in ipairs(rows) do
        if (not only_active) or r.active then
            out[#out + 1] = r                    -- runtime-selected subset -> deferred+promote
        end
    end
    return out
end

return list
