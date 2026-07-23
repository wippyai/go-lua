local pid: string = "worker"
local payload = { id = "runtime-copy" }

process.send(pid, "copy", payload)
local still_local = payload.id
