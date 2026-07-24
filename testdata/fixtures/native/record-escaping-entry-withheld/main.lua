-- The constructed record is sent to another process: escape is not closed, so a
-- specialized storage class for its entries is withheld.

type Job = { id: string, attempts: integer }

local function dispatch(pid: string, id: string)
    local job: Job = { id = id, attempts = 0 }
    process.send(pid, "job.ready", job)
end

return dispatch
