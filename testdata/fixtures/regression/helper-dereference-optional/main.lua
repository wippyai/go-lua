-- Negative control for the rows helper from spiralscout.estimation:projection_fold_test.
local sql = require("sql")

local function rows(db: any, statement: string, params: any, skip: boolean): any
    if skip then return {} end
    local r = db:query(statement, params)
    return r or {}
end

local db = sql.get("spiralscout.estimation:db")
rows(db, "SELECT node_id FROM estimate_node", {}, true)
db:release()
