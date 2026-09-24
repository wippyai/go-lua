-- Negative control for the SQL helper in spiralscout.estimation:projection_fold_test.
local sql = require("sql")
local db = sql.get("spiralscout.estimation:db")
local function rows(value: any): any
    local r = value:query("SELECT node_id FROM estimate_node", {})
    db = nil
    return r
end
rows(db)
db:release()
