local co = coroutine.create(function()
    local payload = {
        id = "yielded",
        meta = {
            route = "worker",
        },
    }
    coroutine.yield(payload)
end)

local ok, yielded = coroutine.resume(co)
if ok then
    process.send("worker-1", "payload.ready", yielded)
end
