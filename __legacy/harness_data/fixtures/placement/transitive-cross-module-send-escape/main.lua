local pass = require("pass")

type Packet = pass.Packet

local packet: Packet = pass.build("x")
process.send("worker-1", "packet.ready", packet)
