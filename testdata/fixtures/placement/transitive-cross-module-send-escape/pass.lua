local alloc = require("alloc")

type Packet = alloc.Packet

local M = {}
M.Packet = Packet

function M.build(id: string): Packet
    return alloc.make(id)
end

return M
