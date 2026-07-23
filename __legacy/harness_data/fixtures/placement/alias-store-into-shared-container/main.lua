local shared = {
    label = "shared",
}

process.send("worker-1", "container.ready", shared)

local payload = {
    id = "stored",
}

local alias = payload
shared.payload = alias

local id: string = alias.id
print(id)
