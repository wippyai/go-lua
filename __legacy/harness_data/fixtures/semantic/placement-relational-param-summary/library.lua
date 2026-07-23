local protocol = require("protocol")

local M = {}
local saved: {[string]: protocol.Item} = {}

function M.read_item(item: protocol.Item): string
    return item.child.meta.route
end

function M.store_item(item: protocol.Item): ()
    saved.last = item
end

return M
