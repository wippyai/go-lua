-- CLEAN positive, distilled from upload_read_model.lua materialized_row.
-- The materialized view is built and returned within the actor -> OwnedHeap, no send.
type Upload = { id: string, size: number, mime: string }
type View = { id: string, human_size: string }

local function materialize(u: Upload): View
    local kb: number = u.size / 1024                              -- no Heap root
    local view: View = { id = u.id, human_size = tostring(kb) }   -- OwnedHeap: returned within actor, never sent
    return view
end

return materialize
