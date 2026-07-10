type Route = {
    id: string,
    next_id: string?,
}

type RouteTable = {
    routes: {[string]: Route},
}

local table_state: RouteTable = {
    routes = {
        start = {id = "start", next_id = "middle"},
        middle = {id = "middle", next_id = "end"},
        ["end"] = {id = "end", next_id = nil},
    },
}

local start = table_state.routes.start
if start and start.next_id then
    local next_route = table_state.routes[start.next_id]
    if next_route then
        local id: string = next_route.id
    end
end

local missing: Route = table_state.routes["missing"] -- expect-error

return "ok"
