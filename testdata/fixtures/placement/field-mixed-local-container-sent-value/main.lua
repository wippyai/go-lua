local payload = {
    id = "payload",
}

local holder = {
    payload = payload,
}

process.send("worker-1", "payload.ready", payload)

local id: string = holder.payload.id
print(id)
