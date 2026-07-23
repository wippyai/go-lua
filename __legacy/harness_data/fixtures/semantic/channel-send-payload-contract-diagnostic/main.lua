type Job = { id: string, meta: { attempt: number } }

local function dispatch(out: Channel<Job>)
    out:send({ id = 1, meta = { attempt = 1 } })
end
