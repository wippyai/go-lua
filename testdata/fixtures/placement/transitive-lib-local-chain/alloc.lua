type Packet = {
    id: string,
    meta: {
        route: string,
    },
}

local M = {}
M.Packet = Packet

function M.make(id: string): Packet
    local packet: Packet = {
        id = id,
        meta = {
            route = "local",
        },
    }
    return packet
end

return M
