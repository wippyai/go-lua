local protocol = require("protocol")

local M = {}

function M.make(): protocol.Node
    return {
        kind = "group",
        children = {
            {kind = "text", value = "a"},
            {kind = "group", children = {}},
        },
    }
end

return M
