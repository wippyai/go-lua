local protocol = require("protocol")
local storage = require("storage")

local box: protocol.Box = {
    label = "owned",
    items = {},
}

local item: protocol.Item = {
    id = "item-1",
    tags = {
        phase = "created",
    },
    child = {
        id = "child-1",
        meta = {
            route = "owned",
            shard = "primary",
        },
    },
}

local scratch = {
    note = {
        route = "local",
    },
}

storage.store_item(item, box)

local stored_route: string = item.child.meta.route
local local_route: string = scratch.note.route
print(stored_route, local_route)
