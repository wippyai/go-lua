local box = {items = {}}
local item = {
    id = "owned",
    child = {
        route = "owned",
    },
}

ownership.store(item, box)

local route: string = item.child.route
print(route)
