local lib = require("lib")

local M = {}

function M.run()
    local box: lib.Boxed = lib.wrap("payload-send")
    process.send("worker", "topic", box)
end

return M
