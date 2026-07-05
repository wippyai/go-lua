local lib = require("lib")

local M = {}

function M.run(): string
    local box: lib.Boxed = lib.wrap("payload-local")
    local body: string = box.body
    return body
end

return M
