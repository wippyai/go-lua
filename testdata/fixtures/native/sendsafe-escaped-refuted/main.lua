-- SEND SAFETY: a payload stored into another owner before the send has escaped.
-- The verdict is REFUTED, not "unknown": the copy is required by proof, and the
-- distinction is what lets the runtime skip re-deciding at the send.
local pid: string = "collector"

local holder = {}
local payload = { id = "p-1" }
ownership.store(payload, holder)

process.send(pid, "payload", payload)

return holder
