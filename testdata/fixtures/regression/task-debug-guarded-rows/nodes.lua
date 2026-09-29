-- keeper.task.persist:nodes_reader
--
-- Typed selectors over keeper_task_nodes. The UI timeline and the tabs
-- (Spec, Findings, Integrations, …) all come out of this file; agents
-- read findings / prior specs through the same API.
--
-- All queries scope to a single task_id. Results are plain tables; the
-- consumer formats for display.

local sql  = require("sql")
local json = require("json")

local task_consts = require("task_consts")

local M = {}

-- ---------------------------------------------------------------------------
-- Internal
-- ---------------------------------------------------------------------------

local function get_db()
    local db, err = sql.get(task_consts.DATABASE.RESOURCE_ID)
    if err then return nil, "nodes_reader db: " .. tostring(err) end
    return db, nil
end

local ROW_COLS =
    "node_id, task_id, parent_node_id, path, depth, position, " ..
    "type, discriminator, title, content, content_type, " ..
    "status, visibility, " ..
    "agent_id, dataflow_id, changeset_id, " ..
    "execution_ms, error_message, result_summary, metadata, " ..
    "seq, created_at, updated_at"

local function decode_row(row)
    if row and type(row.metadata) == "string" and row.metadata ~= "" then
        local decoded, err = json.decode(row.metadata)
        if not err then row.metadata = decoded end
    end
    return row
end

local function decode_rows(rows: unknown): {[integer]: {[string]: unknown}}
    if type(rows) ~= "table" then return {} end
    local out: {[integer]: {[string]: unknown}} = {}
    for _, r in ipairs(rows) do
        if type(r) == "table" then
            table.insert(out, decode_row(r))
        end
    end
    return out
end

local function exec_query(q, params)
    local db, err = get_db()
    if err then return nil, err end
    local rows, qerr = db:query(q, params or {})
    db:release()
    if qerr then return nil, "query: " .. tostring(qerr) end
    return decode_rows(rows), nil
end

-- ---------------------------------------------------------------------------
-- Public API
-- ---------------------------------------------------------------------------

-- All nodes for a task, seq ASC.
function M.list(task_id, opts)
    opts = opts or {}
    local where = { "task_id = ?" }
    local params = { task_id }

    if opts.visibility then
        -- "user" | "user,debug" | ("all" = no filter)
        if opts.visibility ~= "all" then
            local vis = {}
            for v in string.gmatch(opts.visibility, "[^,]+") do
                table.insert(vis, (v:gsub("^%s+", ""):gsub("%s+$", "")))
            end
            if #vis == 1 then
                table.insert(where, "visibility = ?")
                table.insert(params, vis[1])
            elseif #vis > 1 then
                local placeholders = string.rep("?,", #vis):sub(1, -2)
                table.insert(where, "visibility IN (" .. placeholders .. ")")
                for _, v in ipairs(vis) do table.insert(params, v) end
            end
        end
    end

    if opts.since_seq then
        table.insert(where, "seq > ?")
        table.insert(params, opts.since_seq)
    end

    if opts.types and #opts.types > 0 then
        local placeholders = string.rep("?,", #opts.types):sub(1, -2)
        table.insert(where, "type IN (" .. placeholders .. ")")
        for _, t in ipairs(opts.types) do table.insert(params, t) end
    end

    local q = "SELECT " .. ROW_COLS ..
              " FROM keeper_task_nodes WHERE " .. table.concat(where, " AND ") ..
              " ORDER BY seq ASC"
    if opts.limit then
        q = q .. " LIMIT " .. tostring(tonumber(opts.limit))
    end
    return exec_query(q, params)
end

-- Latest node of a given type (e.g. current spec).
function M.latest_of_type(task_id, type_name, opts)
    opts = opts or {}
    local where = { "task_id = ?", "type = ?" }
    local params = { task_id, type_name }

    if opts.status then
        table.insert(where, "status = ?")
        table.insert(params, opts.status)
    end
    if opts.discriminator then
        table.insert(where, "discriminator = ?")
        table.insert(params, opts.discriminator)
    end

    local q = "SELECT " .. ROW_COLS ..
              " FROM keeper_task_nodes WHERE " .. table.concat(where, " AND ") ..
              " ORDER BY seq DESC LIMIT 1"
    local rows, err = exec_query(q, params)
    if err then return nil, err end
    return rows[1], nil
end

-- All rows for a type ordered by seq ASC (e.g. all spec revisions, phase transitions).
function M.by_type(task_id, type_name, opts)
    opts = opts or {}
    local where = { "task_id = ?", "type = ?" }
    local params = { task_id, type_name }

    if opts.status then
        table.insert(where, "status = ?")
        table.insert(params, opts.status)
    end
    if opts.discriminator then
        table.insert(where, "discriminator = ?")
        table.insert(params, opts.discriminator)
    end

    local order = opts.order == "desc" and "DESC" or "ASC"
    local q = "SELECT " .. ROW_COLS ..
              " FROM keeper_task_nodes WHERE " .. table.concat(where, " AND ") ..
              " ORDER BY seq " .. order
    if opts.limit then
        q = q .. " LIMIT " .. tostring(tonumber(opts.limit))
    end
    return exec_query(q, params)
end

-- Findings deduped to the latest row per discriminator (the key).
function M.findings(task_id)
    local q = [[
        SELECT ]] .. ROW_COLS .. [[ FROM keeper_task_nodes t
        WHERE task_id = ? AND type = 'finding'
          AND seq = (
            SELECT MAX(seq) FROM keeper_task_nodes
            WHERE task_id = t.task_id AND type = 'finding' AND discriminator = t.discriminator
          )
        ORDER BY seq ASC
    ]]
    return exec_query(q, { task_id })
end

-- Direct children of a node (useful for integrate tree walk).
function M.children(parent_node_id, opts)
    opts = opts or {}
    local q = "SELECT " .. ROW_COLS ..
              " FROM keeper_task_nodes WHERE parent_node_id = ? ORDER BY position ASC"
    local rows, err = exec_query(q, { parent_node_id })
    if err then return nil, err end
    if opts.limit then
        local trimmed = {}
        for i = 1, math.min(opts.limit, #rows) do trimmed[i] = rows[i] end
        return trimmed, nil
    end
    return rows, nil
end

-- Get by node_id.
function M.get(node_id)
    local q = "SELECT " .. ROW_COLS ..
              " FROM keeper_task_nodes WHERE node_id = ? LIMIT 1"
    local rows, err = exec_query(q, { node_id })
    if err then return nil, err end
    return rows[1], nil
end

-- Count transitions: how many times (from→to) has happened on this task.
-- Drives bounce caps. opts.after_seq lets phase retry budgets reset after a
-- successful progress boundary without deleting the older audit trail.
function M.transition_count(task_id, from_phase, to_phase, opts)
    opts = opts or {}
    local disc = from_phase .. "->" .. to_phase
    local q = [[
        SELECT COUNT(*) AS c FROM keeper_task_nodes
        WHERE task_id = ? AND type = 'phase_transition' AND discriminator = ?
    ]]
    local params = { task_id, disc }
    if opts.after_seq then
        q = q .. " AND seq > ?"
        table.insert(params, tonumber(opts.after_seq) or 0)
    end
    local rows, err = exec_query(q, params)
    if err then return 0, err end
    return tonumber(rows[1] and rows[1].c) or 0, nil
end

function M.latest_transition_seq(task_id, from_phase, to_phase)
    local disc = from_phase .. "->" .. to_phase
    local q = [[
        SELECT MAX(seq) AS seq FROM keeper_task_nodes
        WHERE task_id = ? AND type = 'phase_transition' AND discriminator = ?
    ]]
    local rows, err = exec_query(q, { task_id, disc })
    if err then return nil, err end
    return tonumber(rows[1] and rows[1].seq), nil
end

-- FTS search scoped to a task.
function M.search(task_id, query_text, opts)
    opts = opts or {}
    local q = [[
        SELECT n.node_id, n.task_id, n.type, n.discriminator, n.title, n.created_at, n.seq
        FROM keeper_task_nodes_fts f
        JOIN keeper_task_nodes n ON n.rowid = f.rowid
        WHERE n.task_id = ? AND keeper_task_nodes_fts MATCH ?
        ORDER BY n.seq DESC
    ]]
    if opts.limit then
        q = q .. " LIMIT " .. tostring(tonumber(opts.limit))
    end
    return exec_query(q, { task_id, query_text })
end

-- FTS search across all task nodes.
function M.search_global(query_text, opts)
    opts = opts or {}
    local q = [[
        SELECT n.node_id, n.task_id, n.type, n.discriminator, n.title, n.created_at, n.seq
        FROM keeper_task_nodes_fts f
        JOIN keeper_task_nodes n ON n.rowid = f.rowid
        WHERE keeper_task_nodes_fts MATCH ?
        ORDER BY n.seq DESC
    ]]
    if opts.limit then
        q = q .. " LIMIT " .. tostring(tonumber(opts.limit))
    end
    return exec_query(q, { query_text })
end

return M

