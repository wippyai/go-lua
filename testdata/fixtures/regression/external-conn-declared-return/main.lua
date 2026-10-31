local transport = require("transport")
local function connection_id(): string? return "connection" end

-- Source: kickside.phantombuster.traits:get_tool.
local function run(action: string)
    local conn, err = transport.connect(connection_id())
    if err then
        return "Error: " .. err
    end

    local result
    if action == "get_org" then
        result = transport.get_org(conn)
    end
    return result
end

return run("get_org")
