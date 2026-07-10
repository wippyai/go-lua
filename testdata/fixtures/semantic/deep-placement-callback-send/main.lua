local builder = require("builder")

local batch = builder.build({"route-1", "route-2"}, function(item, id: string, index: number)
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
    local bad_route: number = batch.items["route-1"].child.meta.route -- expect-error
    process.send("worker-1", "route.ready", batch.items["route-1"])
    print(route)
end
