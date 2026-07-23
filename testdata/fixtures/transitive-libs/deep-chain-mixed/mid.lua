local lib = require("lib")

local M = {}
M.Boxed = lib.Boxed

function M.make(payload: string): M.Boxed
    return lib.wrap(payload)
end

return M
