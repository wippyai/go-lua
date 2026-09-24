-- app:value_store_bench (crm test): open_db() returns a present DB because
-- fail() never returns. That must be known in the first fixpoint iteration:
-- a DB? from an iteration that did not know it reaches query()'s parameter
-- hint and stays there, failing db:query and db:execute.
local sql = require("sql")

local M = {}

local APP_DB = "app:db"

local function fail(message)
    error(tostring(message), 2)
end

local function check_err(err, context)
    if err then
        fail((context or "operation failed") .. ": " .. tostring(err))
    end
end

local function execute(db, statement, params, context)
    local _, err = db:execute(statement, params or {})
    check_err(err, context or statement)
end

local function query(db, statement, params, context)
    local rows, err = db:query(statement, params or {})
    check_err(err, context or statement)
    return rows or {}
end

local function open_db()
    local db, err = sql.get(APP_DB)
    check_err(err, "open " .. APP_DB)
    if not db then
        fail(APP_DB .. " unavailable")
    end
    return db
end

local function db_dialect(db)
    local ok, db_type = pcall(function()
        return db:type()
    end)
    if ok and db_type == sql.type.SQLITE then
        return "sqlite"
    end
    return "postgres"
end

local function cleanup(db)
    execute(db, "DELETE FROM spiralscout_crm_record WHERE crm_id = $1", { "s0-bench" }, "cleanup")
end

local function run_q1(db)
    return #query(db, "SELECT 1", {}, "Q1")
end

function M.load(input)
    local db = open_db()
    local ok, err = pcall(function()
        local loops = tonumber(input and input.loops) or 1
        for _ = 1, loops do
            run_q1(db)
        end
    end)
    db:release()
    if not ok then
        return { ok = false, error = tostring(err) }
    end
    return { ok = true }
end

function M.run()
    local db = open_db()
    local dialect = db_dialect(db)
    cleanup(db)
    db:release()
    return dialect
end

return M
