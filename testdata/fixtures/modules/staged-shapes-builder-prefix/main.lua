local routes = require("route_builder")
local service = require("service")

local route = routes.new_route("events")
local name = route.name
local service_kind = route.service_kind
local status = route.status
local label = route.label

return name .. ":" .. service_kind .. ":" .. status .. ":" .. label
