local resource = require("resource")

local function clean_connection_lifecycle()
    local conn = resource.connect()
    resource.query(conn)
    resource.close(conn)
end

local function clean_transaction_lifecycle()
    local conn = resource.connect()
    local tx = resource.begin(conn)
    resource.commit(tx)
    resource.close(conn)
end
