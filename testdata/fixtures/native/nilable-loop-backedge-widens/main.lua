-- A narrowing established inside a loop body does not survive the backedge: the carrier is
-- reassigned from an optional source before the edge, so the loop header must publish
-- maybe_nil and must not carry the body's non_nil row.

local function scan(rows: {string?}): number
    local cur: string? = rows[1]
    local n = 0
    for i = 1, #rows do
        if cur ~= nil then
            n = n + #cur
        end
        cur = rows[i]
    end
    return n
end

return scan
