type FrozenPayload = {
    id: string,
}

local pid: string = "worker"

process.send(pid, "fresh", { id = "fresh" })

local frozen: FrozenPayload = { id = "frozen" }
table.freeze(frozen)
process.send(pid, "frozen", frozen)

local escaped = { id = "escaped" }
local holder = {}
ownership.store(escaped, holder)
process.send(pid, "escaped", escaped)

local unknown = { id = "unknown" }
process.send(pid, "unknown", unknown)
