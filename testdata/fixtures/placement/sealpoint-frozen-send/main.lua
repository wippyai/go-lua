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

-- Luau freeze is shallow. Seal the child before the parent so the complete
-- payload graph, rather than only its root header, is immutable at send.
table.freeze(packet.meta)
table.freeze(packet)
process.send("worker-1", "packet.ready", packet)
