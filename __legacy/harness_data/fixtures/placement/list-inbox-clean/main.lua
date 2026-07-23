-- CLEAN positive, distilled from list_inbox.lua:103-117.
-- Derive-then-drop: nothing escapes; every allocation is scalar/frame-local. Zero promotion.
type Row = { id: string, subject: string, unread: boolean }

local function count_unread(rows: {Row}): number
    local n = 0                          -- Scalar
    for _, r in ipairs(rows) do
        if r.unread then
            n = n + 1
        end
    end
    return n                             -- only a scalar leaves the frame
end

return count_unread
