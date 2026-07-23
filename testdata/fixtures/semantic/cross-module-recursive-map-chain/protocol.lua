type Route = {
    id: string,
    next_id: string?,
}

type RouteTable = {
    routes: {[string]: Route},
}

local M = {}
M.Route = Route
M.RouteTable = RouteTable

return M
