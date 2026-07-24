-- SEND SAFETY: a freshly constructed closed table literal sent directly is
-- proven isolated. The transfer is a pointer move and the runtime copies nothing.
local pid: string = "collector"

process.send(pid, "sample", { id = "s-1", weight = 3 })

return pid
