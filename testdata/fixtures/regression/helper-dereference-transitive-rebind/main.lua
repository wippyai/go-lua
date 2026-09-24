local sql = require("sql")
local db = sql.get("spiralscout.estimation:db")
local function clear() db = nil end
local helpers = {}
function helpers.rows(value: any): any
    local r = value:query("SELECT node_id FROM estimate_node", {})
    clear()
    return r
end
helpers.rows(db)
db:release()
