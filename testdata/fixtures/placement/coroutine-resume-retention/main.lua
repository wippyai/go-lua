type Holder = {
    saved: any?,
}

local holder: Holder = {}

local co = coroutine.create(function(value)
    holder.saved = value
end)

local payload = {
    id = "resume",
    meta = {
        route = "worker",
    },
}

coroutine.resume(co, payload)
