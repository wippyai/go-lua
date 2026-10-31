local sql = require("sql")
local db = sql.get("spiralscout.estimation:db")
local function rows(value: any): any
    local r = value:query("SELECT node_id FROM estimate_node", {})
    db = nil
    return r
end
local helpers = { rows = rows }
helpers.rows(db)
db:release()
