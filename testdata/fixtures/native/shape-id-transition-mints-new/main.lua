-- Adding a field mints a new shape identity: the old id is not reused and the
-- transition edge is published, so the guard lands exactly at the store.

local function build(id: string): string
    local row = { id = id }
    local before = row.id
    row.retries = 0
    local after = row.id
    return before .. after
end

return build
