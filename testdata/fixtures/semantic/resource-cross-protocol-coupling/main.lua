local resource = require("resource")

local function close_before_commit_crosses_protocols()
    local conn = resource.connect()
    local tx = resource.begin(conn)
    resource.close(conn)
    resource.commit(tx)
end

local function commit_before_close_is_clean()
    local conn = resource.connect()
    local tx = resource.begin(conn)
    resource.commit(tx)
    resource.close(conn)
end
