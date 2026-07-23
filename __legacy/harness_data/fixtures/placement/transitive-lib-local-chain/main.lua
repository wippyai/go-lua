local pass = require("pass")

type Packet = pass.Packet

local packet: Packet = pass.build("local")
local route: string = packet.meta.route
print(route)
