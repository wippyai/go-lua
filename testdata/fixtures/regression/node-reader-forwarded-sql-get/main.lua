local sql = require("sql")
local APP_DB = "app:db"

-- Source: userspace.dataflow.persist:node_reader.
local function get_db()
    return sql.get(APP_DB)
end

local db, db_err = get_db()
if db_err then
    return nil, "Failed to connect to database: " .. db_err
end
db:release()
