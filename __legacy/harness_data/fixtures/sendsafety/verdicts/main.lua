local pid: string = "worker"

process.send(pid, "fresh", { id = "fresh" })

local alias = { id = "alias" }
process.send(pid, "alias", alias)
local after_alias = alias.id

local captured = { id = "captured" }
process.send(pid, "closure", { fn = function() return captured.id end })

local module_state = {}
local stored = { id = "stored" }
module_state.payload = stored
ownership.store(stored, module_state)
process.send(pid, "stored", stored)

local sealed = { id = "sealed" }
table.freeze(sealed)
process.send(pid, "sealed", sealed)
