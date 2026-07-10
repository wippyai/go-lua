type Message = {
    id: string,
    deferred: boolean,
    payload: { attempts: number },
}

local function route(pid: string, urgent: boolean, id: string)
    local msg: Message = {
        id = id,
        deferred = false,
        payload = { attempts = 0 },
    }
    if urgent then
        table.freeze(msg)
        process.send(pid, "route.ready", msg)
    else
        msg.deferred = true
    end
end
