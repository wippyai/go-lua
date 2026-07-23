type Meta = {
    route: string,
    shard: string,
}

type Child = {
    id: string,
    meta: Meta,
}

type Item = {
    id: string,
    tags: {[string]: string},
    child: Child,
}

type Batch = {
    items: {[string]: Item},
    count: number,
}

local function build(ids: {string}, fill: (Item, string, number) -> ()): Batch
    local batch: Batch = {items = {}, count = 0}
    for _, id in ipairs(ids) do
        batch.count = batch.count + 1
        local item: Item = {
            id = id,
            tags = {},
            child = {
                id = id,
                meta = {route = "", shard = ""},
            },
        }
        item.tags["phase"] = "constructing"
        fill(item, id, batch.count)
        item.tags["phase"] = "ready"
        batch.items[id] = item
    end
    return batch
end

local batch = build({"route-1", "route-2"}, function(item, id: string, index: number)
    item.child.meta.route = id
    if index == 1 then
        item.child.meta.shard = "primary"
    else
        item.child.meta.shard = "backup"
    end
    item.tags["callback"] = "filled"
end)

if batch.items["route-1"] then
    local route: string = batch.items["route-1"].child.meta.route
    process.send("worker-1", "route.ready", batch.items["route-1"])
    print(route)
end
