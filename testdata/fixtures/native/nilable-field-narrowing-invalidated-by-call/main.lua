-- A narrowed optional field re-widens across an opaque call. Unlike an uncaptured local, the
-- field is reachable by the callee through the record, so call.opaque revokes the narrowing.
type Row = { id: string? }

local function run(r: Row, notify: (Row) -> ()): string
    if r.id == nil then
        return "none"
    end
    local before = r.id
    notify(r)
    return before .. (r.id or "gone")
end

return run
