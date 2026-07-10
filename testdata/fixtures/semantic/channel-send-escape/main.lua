type Job = { id: string, meta: { attempts: number } }
local function dispatch(out: Channel<Job>, id: string)
    local job: Job = { id = id, meta = { attempts = 0 } }
    out:send(job)
end
