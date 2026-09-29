-- Extracted from spiralscout.estimation:projection_fold_test, lines 31-49 and 103-105.
local sql = require("sql")
local test = require("test")
local T = { ACTIVITY = "estimate_activity" }

local function rows(db: any, statement: string, params: any): any
    local r, e = db:query(statement, params)
    test.is_nil(e)
    return r or {}
end

local function count(db: any, table_name: string, estimate_id: string): number
    local r = rows(db, "SELECT COUNT(*) AS n FROM " .. table_name .. " WHERE estimate_id=$1", { estimate_id })
    return tonumber((r[1] :: any).n) or 0
end

local function check_count(eid: string)
    local activity_before = 1
    local db2 = sql.get("spiralscout.estimation:db")
    test.eq(count(db2, T.ACTIVITY, eid), activity_before)
    db2:release()
end

return check_count
