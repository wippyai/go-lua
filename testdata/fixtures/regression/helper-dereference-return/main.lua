-- From spiralscout.estimation:projection_fold_test, state_snapshot and rows.
local sql = require("sql")

local function rows(db: any, statement: string, params: any): any
    local r, e = db:query(statement, params)
    return r or {}
end

local function count(db: any, table_name: string, estimate_id: string): any
    local r = rows(db, "SELECT COUNT(*) FROM " .. table_name .. " WHERE estimate_id=$1", { estimate_id })
    return r
end

local function state_snapshot(estimate_id: string)
    local db = sql.get("spiralscout.estimation:db")
    local node = rows(db, "SELECT node_id FROM estimate_node WHERE estimate_id=$1", { estimate_id })
    db:release()
    local db2 = sql.get("spiralscout.estimation:db")
    local activity_before = count(db2, "estimate_activity", estimate_id)
    db2:release()
    return node, activity_before
end

return state_snapshot
