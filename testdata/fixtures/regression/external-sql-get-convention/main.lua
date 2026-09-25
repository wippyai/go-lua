local sql = require("sql")
local APP_DB = "app:db"

-- Source: userspace.dataflow.persist:node_reader get_db and methods:all.
local direct, direct_err = sql.get(APP_DB)
if direct_err then return end
direct:release()
