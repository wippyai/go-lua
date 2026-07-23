local service = require("service")

local M = {}

function M.new_route(name)
  local route = {}
  route.service_kind = service.kind
  route.name = name

  local alias = route
  local captured_name = route.name
  alias.status = "ready"
  alias.label = captured_name .. ":" .. alias.status

  return route
end

return M
