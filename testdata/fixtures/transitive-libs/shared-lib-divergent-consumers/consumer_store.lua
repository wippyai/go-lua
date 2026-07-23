local lib = require("lib")

local M = {}

local registry = {}

function M.run()
    local box: lib.Boxed = lib.wrap("payload-store")
    ownership.store(box, registry)
end

return M
