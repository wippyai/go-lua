-- An ownership disposition proven for a freshly constructed record is revoked when the
-- record is sent: after escape the construction no longer licenses unguarded mutation.

local pid: string = "worker"

local msg = { id = "m1", attempts = 0 }

process.send(pid, "job", msg)

msg.attempts = 1

return msg.id
