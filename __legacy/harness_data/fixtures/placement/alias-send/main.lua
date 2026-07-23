local packet = {
    id = "shared",
}

local alias = packet
process.send("worker-1", "packet.ready", alias)
