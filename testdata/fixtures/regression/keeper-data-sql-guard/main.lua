-- keeper.agents.tools:data
--
-- Read-only SQL inspector. Discovers registered databases and lets the agent
-- list tables, describe schemas, count rows, sample rows, run read-only
-- queries, and enumerate applied migrations. All output is markdown.
--
-- Writes are rejected: the query action only accepts statements that begin
-- with SELECT (after stripping leading whitespace and comments). Everything
-- else is blocked.

local sql = require("sql")
local json = require("json")
local registry = require("registry")

local render = require("render")

local M = {}

type Params = {[string]: unknown}
type ActionFn = (Params) -> (unknown?, string?)

local function mcp_text(text: string)
    return { _mcp_content = { { type = "text", text = text } } }
end

local ACTIONS: {[string]: ActionFn} = {}

local MAX_LIMIT = 500
local DEFAULT_LIMIT = 50

function M.require_string(params: Params, key: string): (string?, string?)
    local v = params[key]
    if v == nil or v == "" then return nil, key .. " is required" end
    if type(v) ~= "string" then return nil, key .. " must be a string, got " .. type(v) end
    return v
end
local require_string = M.require_string

local function checked_string(params: Params, key: string): (string, string?)
    local value, err = require_string(params, key)
    if err then return "", err end
    if not value then return "", key .. " is required" end
    return value, nil
end

local function dialect_of(db_id: string): string
    local entry, err = registry.get(db_id)
    if err or not entry then return "sqlite" end
    local kind = entry.kind or ""
    if kind:find("sqlite") then return "sqlite" end
    if kind:find("postgres") then return "postgres" end
    if kind:find("mysql") then return "mysql" end
    return "sqlite"
end

local function list_registered_dbs()
    local entries, err = registry.find({ [".kind"] = "db.sql.*" })
    if err then return nil, err end
    local rows = {}
    for _, e in ipairs(entries or {}) do
        table.insert(rows, { id = e.id, kind = e.kind })
    end
    table.sort(rows, function(a, b) return a.id < b.id end)
    return rows
end

function M.reject_non_select(stmt)
    if type(stmt) ~= "string" then return "query must be a string" end
    local s = stmt:gsub("^%s+", "")
    while true do
        if s:sub(1, 2) == "--" then
            local nl = s:find("\n", 1, true)
            s = nl and s:sub(nl + 1) or ""
        elseif s:sub(1, 2) == "/*" then
            local close = s:find("*/", 3, true)
            s = close and s:sub(close + 2) or ""
        else
            break
        end
        s = s:gsub("^%s+", "")
    end
    local head = s:sub(1, 8):lower()
    if head:sub(1, 6) == "select" or head:sub(1, 4) == "with" or head:sub(1, 7) == "explain" or head:sub(1, 6) == "pragma" then
        return nil
    end
    return "only SELECT / WITH / EXPLAIN / PRAGMA statements are allowed"
end
local reject_non_select = M.reject_non_select

-- ---------------------------------------------------------------------------
-- list_dbs
-- ---------------------------------------------------------------------------

function ACTIONS.list_dbs(params)
    local rows, err = list_registered_dbs()
    if err then return nil, "registry.find: " .. tostring(err) end

    local out = {}
    table.insert(out, "# Databases")
    table.insert(out, "")
    if #rows == 0 then
        table.insert(out, "(none registered)")
    else
        table.insert(out, render.table_header({ "db_id", "kind" }))
        for _, r in ipairs(rows) do
            table.insert(out, render.table_row({ r.id, r.kind }))
        end
    end
    table.insert(out, "")
    table.insert(out, "Next: `action=list_tables db=<db_id>`.")
    return mcp_text(table.concat(out, "\n"))
end

-- ---------------------------------------------------------------------------
-- list_tables
-- ---------------------------------------------------------------------------

local function fetch_tables(db, dialect)
    if dialect == "postgres" then
        return db:query([[
            SELECT tablename AS name FROM pg_tables
            WHERE schemaname NOT IN ('pg_catalog','information_schema')
            ORDER BY tablename
        ]])
    end
    if dialect == "mysql" then
        return db:query([[
            SELECT table_name AS name FROM information_schema.tables
            WHERE table_schema = DATABASE() ORDER BY table_name
        ]])
    end
    return db:query([[
        SELECT name FROM sqlite_master
        WHERE type='table' AND name NOT LIKE 'sqlite_%'
        ORDER BY name
    ]])
end

function ACTIONS.list_tables(params)
    local db_id, serr = checked_string(params, "db")
    if serr then return nil, serr end

    local db, err = sql.get(db_id)
    if err then return nil, "sql.get " .. db_id .. ": " .. tostring(err) end

    local dialect = dialect_of(db_id)
    local rows, qerr = fetch_tables(db, dialect)
    if qerr then return nil, "list tables: " .. tostring(qerr) end

    local filter = params.name_filter
    if filter == "" then filter = nil end

    local out = {}
    table.insert(out, "# Tables in " .. db_id .. " (" .. dialect .. ")")
    table.insert(out, "")
    table.insert(out, render.table_header({ "table", "rows" }))
    local shown = 0
    for _, r in ipairs(rows or {}) do
        local name = r.name or r.tablename or ""
        if not filter or name:find(filter, 1, true) then
            local cnt = "?"
            local cr, cerr = db:query("SELECT COUNT(*) AS c FROM " .. name)
            if not cerr and cr and cr[1] then cnt = tostring(cr[1].c or cr[1].count or "?") end
            table.insert(out, render.table_row({ name, cnt }))
            shown = shown + 1
        end
    end
    if shown == 0 then
        table.insert(out, render.table_row({ "(none)", "" }))
    end
    table.insert(out, "")
    table.insert(out, "Next: `action=describe db=" .. db_id .. " table=<name>`.")
    return mcp_text(table.concat(out, "\n"))
end

-- ---------------------------------------------------------------------------
-- describe
-- ---------------------------------------------------------------------------

local function fetch_columns(db, dialect, table_name)
    if dialect == "postgres" then
        return db:query([[
            SELECT column_name AS name, data_type AS type,
                   is_nullable AS nullable, column_default AS dflt
            FROM information_schema.columns WHERE table_name = ?
            ORDER BY ordinal_position
        ]], { table_name })
    end
    if dialect == "mysql" then
        return db:query([[
            SELECT column_name AS name, column_type AS type,
                   is_nullable AS nullable, column_default AS dflt
            FROM information_schema.columns
            WHERE table_schema = DATABASE() AND table_name = ?
            ORDER BY ordinal_position
        ]], { table_name })
    end
    return db:query("PRAGMA table_info(" .. table_name .. ")")
end

local function fetch_indexes(db, dialect, table_name)
    if dialect == "sqlite" then
        return db:query("PRAGMA index_list(" .. table_name .. ")")
    end
    return {}, nil
end

function ACTIONS.describe(params)
    local db_id, serr = checked_string(params, "db")
    if serr then return nil, serr end
    local table_name, terr = checked_string(params, "table")
    if terr then return nil, terr end

    local db, err = sql.get(db_id)
    if err then return nil, "sql.get: " .. tostring(err) end
    local dialect = dialect_of(db_id)

    local cols, cerr = fetch_columns(db, dialect, table_name)
    if cerr then return nil, "describe: " .. tostring(cerr) end

    local out = {}
    table.insert(out, "# " .. db_id .. " / " .. table_name)
    table.insert(out, "")
    table.insert(out, "## Columns")

    if dialect == "sqlite" then
        table.insert(out, render.table_header({ "cid", "name", "type", "notnull", "dflt", "pk" }))
        for _, c in ipairs(cols or {}) do
            table.insert(out, render.table_row({
                tostring(c.cid or ""), tostring(c.name or ""), tostring(c.type or ""),
                tostring(c.notnull or 0), tostring(c.dflt_value or ""), tostring(c.pk or 0),
            }))
        end
    else
        table.insert(out, render.table_header({ "name", "type", "nullable", "default" }))
        for _, c in ipairs(cols or {}) do
            table.insert(out, render.table_row({
                tostring(c.name or ""), tostring(c.type or ""),
                tostring(c.nullable or ""), tostring(c.dflt or ""),
            }))
        end
    end

    local idx, ierr = fetch_indexes(db, dialect, table_name)
    if not ierr and type(idx) == "table" and #idx > 0 then
        table.insert(out, "")
        table.insert(out, "## Indexes")
        table.insert(out, render.table_header({ "seq", "name", "unique", "origin", "partial" }))
        for _, i in ipairs(idx) do
            table.insert(out, render.table_row({
                tostring(i.seq or ""), tostring(i.name or ""),
                tostring(i.unique or 0), tostring(i.origin or ""), tostring(i.partial or 0),
            }))
        end
    end

    table.insert(out, "")
    table.insert(out, "Next: `action=sample db=" .. db_id .. " table=" .. table_name .. " limit=10` or `action=count`.")
    return mcp_text(table.concat(out, "\n"))
end

-- ---------------------------------------------------------------------------
-- count
-- ---------------------------------------------------------------------------

function ACTIONS.count(params)
    local db_id, serr = checked_string(params, "db")
    if serr then return nil, serr end
    local table_name, terr = checked_string(params, "table")
    if terr then return nil, terr end

    local db, err = sql.get(db_id)
    if err then return nil, "sql.get: " .. tostring(err) end

    local q = "SELECT COUNT(*) AS c FROM " .. table_name
    local wbinds = {}
    if params.where and params.where ~= "" then
        local werr = reject_non_select("SELECT " .. params.where)
        if werr then return nil, "where: " .. werr end
        q = q .. " WHERE " .. params.where
    end
    local rows, qerr = db:query(q, wbinds)
    if qerr then return nil, "count: " .. tostring(qerr) end
    local c = (rows and rows[1] and (rows[1].c or rows[1].count)) or 0

    local out = {}
    table.insert(out, "# count " .. db_id .. "/" .. table_name)
    if params.where then table.insert(out, "WHERE: `" .. params.where .. "`") end
    table.insert(out, "")
    table.insert(out, "**" .. tostring(c) .. "** rows")
    return mcp_text(table.concat(out, "\n"))
end

-- ---------------------------------------------------------------------------
-- sample
-- ---------------------------------------------------------------------------

local format_rows_as_table
function M.format_rows_as_table(rows)
    if #rows == 0 then return { "(no rows)" } end
    local cols = {}
    local seen = {}
    for _, r in ipairs(rows) do
        for k in pairs(r) do
            if not seen[k] then seen[k] = true; table.insert(cols, k) end
        end
    end
    table.sort(cols)
    local out = { render.table_header(cols) }
    for _, r in ipairs(rows) do
        local cells = {}
        for _, c in ipairs(cols) do
            local v = r[c]
            if type(v) == "table" then
                local ok, enc = pcall(json.encode, v)
                v = ok and enc or ""
            end
            table.insert(cells, render.clip(tostring(v == nil and "" or v), 120))
        end
        table.insert(out, render.table_row(cells))
    end
    return out
end
format_rows_as_table = M.format_rows_as_table

function ACTIONS.sample(params)
    local db_id, serr = checked_string(params, "db")
    if serr then return nil, serr end
    local table_name, terr = checked_string(params, "table")
    if terr then return nil, terr end

    local limit = tonumber(params.limit) or DEFAULT_LIMIT
    if limit > MAX_LIMIT then limit = MAX_LIMIT end
    if limit < 1 then limit = 1 end

    local cols = params.columns or "*"
    local order = params.order_by
    local where = params.where

    if cols ~= "*" then
        local cerr = reject_non_select("SELECT " .. cols)
        if cerr then return nil, "columns: " .. cerr end
    end
    if where and where ~= "" then
        local werr = reject_non_select("SELECT " .. where)
        if werr then return nil, "where: " .. werr end
    end

    local q = "SELECT " .. cols .. " FROM " .. table_name
    if where and where ~= "" then q = q .. " WHERE " .. where end
    if order and order ~= "" then
        local oerr = reject_non_select("SELECT " .. order)
        if oerr then return nil, "order_by: " .. oerr end
        q = q .. " ORDER BY " .. order
    end
    q = q .. " LIMIT " .. limit

    local db, err = sql.get(db_id)
    if err then return nil, "sql.get: " .. tostring(err) end
    local rows, qerr = db:query(q)
    if qerr then return nil, "sample: " .. tostring(qerr) end

    local out = {}
    table.insert(out, "# sample " .. db_id .. "/" .. table_name)
    table.insert(out, "```sql")
    table.insert(out, q)
    table.insert(out, "```")
    table.insert(out, "")
    for _, line in ipairs(format_rows_as_table(rows or {})) do table.insert(out, line) end
    table.insert(out, "")
    table.insert(out, "Returned " .. #rows .. " row(s). Use `limit=`, `order_by=`, `where=` to narrow.")
    return mcp_text(table.concat(out, "\n"))
end

-- ---------------------------------------------------------------------------
-- query (read-only)
-- ---------------------------------------------------------------------------

function ACTIONS.query(params)
    local db_id, serr = checked_string(params, "db")
    if serr then return nil, serr end
    local stmt, merr = checked_string(params, "sql")
    if merr then return nil, merr end

    local reject = reject_non_select(stmt)
    if reject then return nil, reject end

    local limit = tonumber(params.limit) or MAX_LIMIT
    if limit > MAX_LIMIT then limit = MAX_LIMIT end

    local db, err = sql.get(db_id)
    if err then return nil, "sql.get: " .. tostring(err) end
    local query_params = {}
    if type(params.params) == "table" then query_params = params.params end
    local rows, qerr = db:query(stmt, query_params)
    if qerr then return nil, "query: " .. tostring(qerr) end

    local capped = {}
    for i, r in ipairs(rows or {}) do
        if i > limit then break end
        table.insert(capped, r)
    end

    local out = {}
    table.insert(out, "# query " .. db_id)
    table.insert(out, "```sql")
    table.insert(out, stmt)
    table.insert(out, "```")
    table.insert(out, "")
    for _, line in ipairs(format_rows_as_table(capped)) do table.insert(out, line) end
    table.insert(out, "")
    local total = #(rows or {})
    if total > limit then
        table.insert(out, string.format("Showing %d of %d rows (limit=%d).", #capped, total, limit))
    else
        table.insert(out, "Returned " .. total .. " row(s).")
    end
    return mcp_text(table.concat(out, "\n"))
end

-- ---------------------------------------------------------------------------
-- migrations
-- ---------------------------------------------------------------------------

function ACTIONS.migrations(params)
    local db_id, serr = checked_string(params, "db")
    if serr then return nil, serr end

    local db, err = sql.get(db_id)
    if err then return nil, "sql.get: " .. tostring(err) end

    local rows, qerr = db:query([[
        SELECT name FROM sqlite_master
        WHERE type='table' AND name LIKE '%migration%'
    ]])
    if qerr then return nil, "scan migration tables: " .. tostring(qerr) end

    local out = {}
    table.insert(out, "# Migrations for " .. db_id)
    table.insert(out, "")
    if not rows or #rows == 0 then
        table.insert(out, "(no migration tables detected)")
        return mcp_text(table.concat(out, "\n"))
    end

    for _, r in ipairs(rows) do
        local tbl = r.name
        table.insert(out, "## " .. tbl)
        local applied, aerr = db:query("SELECT * FROM " .. tbl .. " ORDER BY 1 DESC LIMIT 50")
        if aerr then
            table.insert(out, "(error: " .. tostring(aerr) .. ")")
        else
            for _, line in ipairs(format_rows_as_table(applied or {})) do table.insert(out, line) end
        end
        table.insert(out, "")
    end

    -- Pending: scan registry for meta.type=migration targeting this db
    local pending, perr = registry.find({ meta = { type = "migration", target_db = db_id } })
    if not perr and type(pending) == "table" then
        table.insert(out, "## Registered (target_db=" .. db_id .. ")")
        table.insert(out, render.table_header({ "entry_id" }))
        for _, e in ipairs(pending) do
            table.insert(out, render.table_row({ e.id }))
        end
    end

    return mcp_text(table.concat(out, "\n"))
end

-- ---------------------------------------------------------------------------
-- dispatcher
-- ---------------------------------------------------------------------------

local function handler(raw_params: unknown)
    local params: Params = {}
    if type(raw_params) == "table" then
        for k, v in pairs(raw_params) do
            if type(k) == "string" then params[k] = v end
        end
    end
    local action = params.action
    if type(action) ~= "string" or action == "" then
        return nil, "action is required"
    end
    local fn = ACTIONS[action]
    if not fn then return nil, "unknown action: " .. tostring(action) end

    local result, err = fn(params)
    if err then return nil, err end

    return result
end

M.handler = handler
return M

