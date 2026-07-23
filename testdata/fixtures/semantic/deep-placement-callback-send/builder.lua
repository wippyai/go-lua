local protocol = require("protocol")

local M = {}

function M.build(ids: {string}, fill: (protocol.Item, string, number) -> ()): protocol.Batch
    local batch: protocol.Batch = {items = {}, count = 0}
    for _, id in ipairs(ids) do
        batch.count = batch.count + 1
        local item: protocol.Item = {
            id = id,
            tags = {},
            child = {
                id = id,
                meta = {route = "", shard = ""},
            },
        }
        item.tags["phase"] = "constructing"
        fill(item, id, batch.count)
        item.tags["phase"] = "ready"
        batch.items[id] = item
    end
    return batch
end

return M
