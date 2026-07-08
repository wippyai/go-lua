type Packet = {
    id: string,
    meta: {
        attempts: number,
    },
}

local packet: Packet = {
    id = "post-send",
    meta = {
        attempts = 1,
    },
}

process.send("worker-1", "packet.ready", packet)
packet.meta.attempts = packet.meta.attempts + 1
