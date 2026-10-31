-- keeper.task.persist:nodes_writer
--
-- Fluent writer for keeper_task_nodes. Every tool/phase/handler event and
-- every durable task payload (spec / finding / integrate stage …) lands here.
-- One table, typed via (type, discriminator), hierarchical via parent_node_id.
--
-- Shape ported from old keeper's design_workspace_data writer:
--   writer.for_task(task_id):node(parent_id):add({type, discriminator, …})
--
-- When ctx.task_id is absent the public API is still callable but every
-- write is a no-op — tools that run outside a task (admin CLI, standalone
-- tests) don't error and don't create orphan rows.

local sql  = require("sql")
local json = require("json")
local uuid = require("uuid")
local time = require("time")

local task_consts = require("task_consts")
local notify = require("notify")

local M = {}

type NodeRow = {
    node_id: string,
    task_id: string,
    path?: string,
    depth?: integer,
    position?: integer,
    seq?: integer,
    status?: string,
    visibility?: string,
    discriminator?: string,
    content?: string,
    execution_ms?: integer,
    result_summary?: string,
}

type TaskNodeHandle = {
    add: (TaskNodeHandle, {[string]: unknown}) -> (NodeRow, string?),
    open: (TaskNodeHandle, {[string]: unknown}) -> (TaskNodeHandle, string?, NodeRow),
}

type TaskNodeWorkspace = {
    task_id: string,
    record: (TaskNodeWorkspace, {[string]: unknown}) -> (NodeRow, string?),
    node: (TaskNodeWorkspace, string?) -> TaskNodeHandle,
    update: (TaskNodeWorkspace, string, {[string]: unknown}) -> (unknown?, string?),
}

-- CQRS event publish. Best-effort broadcast over the admin events bus on the
-- keeper.task topic so subscribed admins (FE relay, dataflow listeners) can react
-- to node creates/updates without polling. Failures are swallowed —
-- persistence must not depend on the bus being up.
local function publish(event, data)
    pcall(function()
        notify.publish(task_consts.TOPIC, { event = event, data = data })
    end)
end

-- ---------------------------------------------------------------------------
-- Low-level helpers
-- ---------------------------------------------------------------------------

local function get_db()
    local db, err = sql.get(task_consts.DATABASE.RESOURCE_ID)
    if err then return nil, "task_nodes_writer db: " .. tostring(err) end
    return db, nil
end

local function now_ms()
    return math.floor(time.now():unix_nano() / 1e6)
end

local function new_id()
    local id, err = uuid.v7()
    if err then return nil, "uuid: " .. tostring(err) end
    return id, nil
end

local function encode_metadata(meta)
    if meta == nil then return "{}", nil end
    if type(meta) == "string" then return meta, nil end
    local s, err = json.encode(meta)
    if err then return nil, "metadata encode: " .. tostring(err) end
    return s, nil
end

-- ---------------------------------------------------------------------------
-- Path / position resolution
-- ---------------------------------------------------------------------------

local function resolve_parent(tx, parent_node_id)
    if not parent_node_id or parent_node_id == "" then
        return { path = "/", depth = 0 }, nil
    end
    local rows, err = tx:query(
        "SELECT path, depth FROM keeper_task_nodes WHERE node_id = ? LIMIT 1",
        { parent_node_id }
    )
    if err then return nil, "parent lookup: " .. tostring(err) end
    if not rows or #rows == 0 then
        return nil, "parent node not found: " .. parent_node_id
    end
    local parent_path = rows[1].path or "/"
    local parent_depth = tonumber(rows[1].depth) or 0
    local new_path = (parent_path == "/" and "/" or parent_path .. "/") .. parent_node_id
    return { path = new_path, depth = parent_depth + 1 }, nil
end

local function next_position(tx, task_id, parent_node_id)
    local q, params
    if parent_node_id and parent_node_id ~= "" then
        q = "SELECT COALESCE(MAX(position), -1) + 1 AS pos FROM keeper_task_nodes WHERE task_id = ? AND parent_node_id = ?"
        params = { task_id, parent_node_id }
    else
        q = "SELECT COALESCE(MAX(position), -1) + 1 AS pos FROM keeper_task_nodes WHERE task_id = ? AND parent_node_id IS NULL"
        params = { task_id }
    end
    local rows, _ = tx:query(q, params)
    if not rows or #rows == 0 then return 0 end
    return tonumber(rows[1].pos) or 0
end

local function next_seq(tx, task_id)
    local rows, _ = tx:query(
        "SELECT COALESCE(MAX(seq), 0) + 1 AS s FROM keeper_task_nodes WHERE task_id = ?",
        { task_id }
    )
    if not rows or #rows == 0 then return 1 end
    return tonumber(rows[1].s) or 1
end

-- ---------------------------------------------------------------------------
-- Core insert
-- ---------------------------------------------------------------------------

-- spec fields:
--   task_id (required)
--   parent_node_id (optional)
--   type (required)
--   discriminator (optional)
--   title, content, content_type (optional)
--   status (optional; one of "running" | "passed" | "failed" | "active" | "superseded")
--   visibility ("user" | "debug" | "internal"; default "user")
--   agent_id, dataflow_id, changeset_id (optional)
--   execution_ms, error_message, result_summary (optional)
--   metadata (table, optional)
local function insert_node(spec)
    if not spec or not spec.task_id or spec.task_id == "" then
        return nil, "task_id required"
    end
    if not spec.type or spec.type == "" then
        return nil, "type required"
    end

    local node_id = spec.node_id
    if not node_id or node_id == "" then
        local id, id_err = new_id()
        if id_err then return nil, id_err end
        node_id = id
    end

    local meta_str, meta_err = encode_metadata(spec.metadata)
    if meta_err then return nil, meta_err end

    local db, db_err = get_db()
    if db_err then return nil, db_err end

    local tx, txerr = db:begin()
    if txerr then db:release(); return nil, "begin: " .. tostring(txerr) end

    local rollback = function(msg)
        tx:rollback(); db:release()
        return nil, msg
    end

    local parent_info, perr = resolve_parent(tx, spec.parent_node_id)
    if perr then return rollback(perr) end
    local position = next_position(tx, spec.task_id, spec.parent_node_id)
    local seq = next_seq(tx, spec.task_id)
    local ts = now_ms()

    local _, ierr = tx:execute([[
        INSERT INTO keeper_task_nodes (
            node_id, task_id, parent_node_id, path, depth, position,
            type, discriminator,
            title, content, content_type,
            status, visibility,
            agent_id, dataflow_id, changeset_id,
            execution_ms, error_message, result_summary,
            metadata,
            seq, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ]], {
        node_id, spec.task_id, spec.parent_node_id, parent_info.path, parent_info.depth, position,
        spec.type, spec.discriminator,
        spec.title or "", spec.content, spec.content_type or "text/plain",
        spec.status, spec.visibility or "user",
        spec.agent_id, spec.dataflow_id, spec.changeset_id,
        spec.execution_ms, spec.error_message, spec.result_summary,
        meta_str,
        seq, ts, ts,
    })
    if ierr then return rollback("insert: " .. tostring(ierr)) end

    local _, cerr = tx:commit()
    db:release()
    if cerr then return nil, "commit: " .. tostring(cerr) end

    publish(task_consts.EVENTS.NODE_CREATED, {
        node_id        = node_id,
        task_id        = spec.task_id,
        parent_node_id = spec.parent_node_id,
        type           = spec.type,
        discriminator  = spec.discriminator,
        status         = spec.status,
        seq            = seq,
    })

    return {
        node_id = node_id,
        task_id = spec.task_id,
        parent_node_id = spec.parent_node_id,
        path = parent_info.path,
        depth = parent_info.depth,
        position = position,
        seq = seq,
        created_at = ts,
    }, nil
end

-- Update subset of fields on an existing node. Useful for turning
-- status=running into status=passed|failed after a tool body returns.
local function update_node(node_id, fields)
    if not node_id or node_id == "" then return nil, "node_id required" end
    if not fields or next(fields) == nil then return nil, "no fields to update" end

    local allowed = {
        title = true, content = true, content_type = true,
        status = true, visibility = true,
        agent_id = true, dataflow_id = true, changeset_id = true,
        execution_ms = true, error_message = true, result_summary = true,
        discriminator = true, type = true,
    }

    local sets, params = { "updated_at = ?" }, { now_ms() }
    for k, v in pairs(fields) do
        if allowed[k] then
            table.insert(sets, k .. " = ?")
            table.insert(params, v)
        end
    end

    if fields.metadata ~= nil then
        -- Merge with the existing metadata so a caller can set just one key
        -- without clobbering the rest (e.g. step_block adds blocker_node_id
        -- without losing kind/needs/target the planner wrote).
        local merged = fields.metadata
        if type(merged) == "table" then
            local db_read, rerr = get_db()
            if not rerr then
                local rows, qerr = db_read:query(
                    "SELECT metadata FROM keeper_task_nodes WHERE node_id = ?",
                    { node_id }
                )
                db_read:release()
                local row = rows and rows[1]
                local metadata = row and row.metadata
                if not qerr and type(metadata) == "string" and metadata ~= "" then
                    local prev, perr = json.decode(metadata)
                    if not perr and type(prev) == "table" then
                        for k, v in pairs(merged) do prev[k] = v end
                        merged = prev
                    end
                end
            end
        end
        local m, merr = encode_metadata(merged)
        if merr then return nil, merr end
        table.insert(sets, "metadata = ?")
        table.insert(params, m)
    end

    if #sets == 1 then return nil, "no allowed fields provided" end

    table.insert(params, node_id)
    local q = "UPDATE keeper_task_nodes SET " .. table.concat(sets, ", ") .. " WHERE node_id = ?"

    local db, db_err = get_db()
    if db_err then return nil, db_err end
    local _, err = db:execute(q, params)
    db:release()
    if err then return nil, "update: " .. tostring(err) end

    publish(task_consts.EVENTS.NODE_UPDATED, {
        node_id = node_id,
        status  = fields.status,
    })

    return { node_id = node_id, updated = true }, nil
end

-- ---------------------------------------------------------------------------
-- Public API
-- ---------------------------------------------------------------------------

-- task_writer.record(spec) -> {node_id, …}, err
--   Thin wrapper when the caller already has task_id.
function M.record(spec)
    return insert_node(spec)
end

-- task_writer.update(node_id, fields) -> {node_id}, err
function M.update(node_id, fields)
    return update_node(node_id, fields)
end

-- task_writer.for_task(task_id) -> workspace handle with fluent node ops
function M.for_task(task_id)
    if not task_id or task_id == "" then
        return nil, "task_id required"
    end

    local ws = {
        task_id = task_id,
    }

    -- ws:record(spec)  — spec gets task_id stamped in automatically
    function ws:record(spec)
        spec = spec or {}
        spec.task_id = task_id
        return insert_node(spec)
    end

    -- ws:node(parent_node_id) -> node handle rooted at parent
    function ws:node(parent_node_id)
        local node = {
            task_id = task_id,
            parent_node_id = parent_node_id,
        }
        function node:add(spec)
            spec = spec or {}
            spec.task_id = task_id
            spec.parent_node_id = parent_node_id
            return insert_node(spec)
        end
        -- Chain: returns a node handle rooted at the just-inserted child.
        function node:open(spec)
            local row, err = node:add(spec)
            if err then return nil, err end
            return ws:node(row.node_id), nil, row
        end
        return node
    end

    -- ws:update(node_id, fields)
    function ws:update(node_id, fields)
        return update_node(node_id, fields)
    end

    return ws :: TaskNodeWorkspace, nil
end

return M
