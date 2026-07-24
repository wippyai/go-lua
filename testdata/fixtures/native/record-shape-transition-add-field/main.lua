-- Assigning a new field after construction is a shape transition: the old and new
-- shapes, the same-object policy and the new storage offset are all published.

local function build(id: string)
    local row = { id = id }
    row.retries = 0
    return row
end

return build
