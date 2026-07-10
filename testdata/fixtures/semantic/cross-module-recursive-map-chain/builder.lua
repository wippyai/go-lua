local protocol = require("protocol")

local M = {}

function M.make(): protocol.RouteTable
    return {
        routes = {
            start = {id = "start", next_id = "end"},
            ["end"] = {id = "end", next_id = nil},
        },
    }
end

return M
