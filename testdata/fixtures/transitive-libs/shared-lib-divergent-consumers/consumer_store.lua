local lib = require("lib")

local M = {}

local registry = {}

function M.run()
    local box: lib.Boxed = lib.wrap("payload-store")
    table.insert(registry, box)
end

return M
