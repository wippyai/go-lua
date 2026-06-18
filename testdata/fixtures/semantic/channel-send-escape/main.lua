type Job = { id: string, attempts: number }
local function dispatch(out: Channel<Job>, id: string)
    local job: Job = { id = id, attempts = 0 }
    out:send(job)
end
