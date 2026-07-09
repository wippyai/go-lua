local protocol = require("protocol")
local library = require("library")

local read_item: protocol.Item = {
    id = "read",
    child = {
        id = "read-child",
        meta = {
            route = "local",
            shard = "a",
        },
    },
}

local stored_item: protocol.Item = {
    id = "stored",
    child = {
        id = "stored-child",
        meta = {
            route = "owned",
            shard = "b",
        },
    },
}

local read_route: string = library.read_item(read_item)
library.store_item(stored_item)
local stored_route: string = stored_item.child.meta.route

print(read_route .. stored_route)
