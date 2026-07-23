local protocol = require("protocol")

local M = {}
local store_item = ownership.store
M.store_item = store_item
M.Item = protocol.Item
M.Box = protocol.Box

return M
