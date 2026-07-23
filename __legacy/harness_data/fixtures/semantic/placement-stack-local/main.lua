local packet = {
    id = "local",
    child = {
        route = "local",
    },
}

local route: string = packet.child.route
print(route)
