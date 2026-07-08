type Packet = {
    id: string,
    meta: {
        route: string,
    },
}

local packet: Packet = {
    id = "sealed",
    meta = {
        route = "frozen",
    },
}

table.freeze(packet)
process.send("worker-1", "packet.ready", packet)
