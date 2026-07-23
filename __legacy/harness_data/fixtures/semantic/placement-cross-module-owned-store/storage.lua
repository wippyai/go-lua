local protocol = require("protocol")

local M = {}

function M.store_item(item: protocol.Item, box: protocol.Box)
    ownership.store(item, box)
end

return M
