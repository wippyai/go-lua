local builder = require("builder")

local state = builder.make()
local start = state.routes.start
if start and start.next_id then
    local next_route = state.routes[start.next_id]
    if next_route then
        local id: string = next_route.id
    end
end

local missing = state.routes["missing"]
if missing then
    local id: string = missing.id
end

return "ok"
