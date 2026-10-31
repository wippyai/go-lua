local sql = require("sql")
local json = require("json")
local uuid = require("uuid")
local time = require("time")
local registry = require("registry")

local consts = require("kb_consts")
local notify = require("notify")

local M = {}

M.TYPE = consts.NODE_TYPE
M.SOURCE = consts.SOURCE

local function get_db()
    local db_id = consts.db_id()
    local db, err = sql.get(db_id)
    if not db then error("database " .. db_id .. " is not available: " .. tostring(err or "unknown")) end
    return db
end

local function now_iso()
    return time.now():format("2006-01-02T15:04:05Z")
end

local function publish(event, data)
    pcall(function()
        notify.publish(consts.TOPIC, { event = event, data = data })
    end)
end

local function parse_json_field(val, default)
    if not val or val == "" then return default end
    local result, err = json.decode(val)
    if err then return default end
    return result
end

local function db_type(db)
    local ok, t = pcall(function() return db:type() end)
    if ok then return t end
    return ""
end

local function is_postgres(db)
    return db_type(db) == sql.type.POSTGRES
end

local function nil_to_null(v)
    if v == nil then return sql.NULL end
    return v
end

local function truthy(v)
    return v == true or v == 1 or v == "1"
end

local function query_one(builder, db)
    local rows, err = builder:limit(1):run_with(db):query()
    if err or not rows or #rows == 0 then return nil, err end
    return rows[1], nil
end

local function decode_embedding(value)
    if type(value) == "table" then return value end
    if type(value) ~= "string" or value == "" then return nil end
    local decoded, err = json.decode(value)
    if err or type(decoded) ~= "table" then return nil end
    return decoded
end

local function cosine_distance(a, b)
    a = decode_embedding(a)
    b = decode_embedding(b)
    if not a or not b then return nil end

    local dot, norm_a, norm_b = 0, 0, 0
    local n = math.min(#a, #b)
    if n == 0 then return nil end
    for i = 1, n do
        local av = tonumber(a[i]) or 0
        local bv = tonumber(b[i]) or 0
        dot = dot + av * bv
        norm_a = norm_a + av * av
        norm_b = norm_b + bv * bv
    end
    if norm_a == 0 or norm_b == 0 then return nil end
    return 1 - (dot / (math.sqrt(norm_a) * math.sqrt(norm_b)))
end

local function compare_distance(a, b)
    return (a.distance or math.huge) < (b.distance or math.huge)
end

-- KB operations

function M.create_kb(params)
    if not params.name or params.name == "" then return nil, "KB name is required" end
    local db = get_db()
    local id = uuid.v4()
    local ts = now_iso()

    local _, err = sql.builder.insert("keeper_kbs")
        :set_map({ id = id, name = params.name, description = params.description or "", created_at = ts, updated_at = ts })
        :run_with(db)
        :exec()
    if err then
        local err_str = tostring(err)
        if err_str:find("UNIQUE constraint") or err_str:find("duplicate key") then
            return nil, "KB with name '" .. params.name .. "' already exists"
        end
        return nil, "Failed to create KB: " .. err_str
    end

    local kb = { id = id, name = params.name, description = params.description or "", created_at = ts, updated_at = ts }
    publish(consts.EVENTS.KB_CREATED, kb)
    return kb
end

function M.list_kbs()
    local db = get_db()
    local rows, err = db:query([[
        SELECT k.*, COUNT(n.id) as node_count
        FROM keeper_kbs k LEFT JOIN keeper_kb_nodes n ON n.kb_id = k.id
        GROUP BY k.id ORDER BY k.name
    ]])
    if err then return nil, "Failed to list KBs: " .. err end

    local kbs = {}
    for _, row in ipairs(rows or {}) do
        table.insert(kbs, {
            id = row.id,
            name = row.name,
            description = row.description,
            node_count = row.node_count or 0,
            created_at = row.created_at,
            updated_at = row.updated_at,
        })
    end
    return kbs
end

function M.get_kb(id)
    local db = get_db()
    local row = query_one(sql.builder.select("*"):from("keeper_kbs"):where("id = ?", id), db)
    return row
end

function M.get_kb_by_name(name)
    local db = get_db()
    local row = query_one(sql.builder.select("*"):from("keeper_kbs"):where("name = ?", name), db)
    return row
end

function M.resolve_kb(name_or_id)
    if not name_or_id or name_or_id == "" then return nil end
    local kb = M.get_kb_by_name(name_or_id)
    if kb then return kb end
    return M.get_kb(name_or_id)
end

function M.update_kb(id, params)
    local db = get_db()
    local ts = now_iso()
    local values = {}

    if params.name ~= nil then values.name = params.name end
    if params.description ~= nil then values.description = params.description end

    if next(values) == nil then return nil, "No fields to update" end

    values.updated_at = ts
    local _, err = sql.builder.update("keeper_kbs")
        :set_map(values)
        :where("id = ?", id)
        :run_with(db)
        :exec()
    if err then return nil, "Failed to update KB: " .. err end

    publish(consts.EVENTS.KB_UPDATED, { id = id })
    return { id = id, updated_at = ts }
end

function M.delete_kb(id)
    if id == consts.DEFAULT_KB_ID then return nil, "Cannot delete the default knowledge base" end
    local db = get_db()
    local kb = M.get_kb(id)
    if not kb then return nil, "KB not found" end

    -- Cascade: delete all nodes in this KB (triggers FTS/embedding cleanup)
    local nodes = sql.builder.select("id")
        :from("keeper_kb_nodes")
        :where("kb_id = ?", id)
        :run_with(db)
        :query()
    for _, row in ipairs(nodes or {}) do
        M.delete(row.id)
    end

    local _, err = sql.builder.delete("keeper_kbs")
        :where("id = ?", id)
        :run_with(db)
        :exec()
    if err then return nil, "Failed to delete KB: " .. err end

    publish(consts.EVENTS.KB_DELETED, { id = id, name = kb.name })
    return { id = id, name = kb.name, deleted = true }
end

-- Node operations

local function row_to_node(row)
    if not row then return nil end
    return {
        id = row.id,
        kb_id = row.kb_id,
        parent_id = row.parent_id,
        workspace_id = row.workspace_id,
        node_type = row.node_type,
        title = row.title,
        content = row.content,
        source = row.source,
        summary = row.summary or "",
        scope_namespace = row.scope_namespace,
        scope_kind = row.scope_kind,
        scope_meta_type = row.scope_meta_type,
        confidence = row.confidence,
        embedded = truthy(row.embedded),
        refs = parse_json_field(row.refs, {}),
        metadata = parse_json_field(row.metadata, {}),
        created_at = row.created_at,
        updated_at = row.updated_at,
    }
end

function M.find_by_title(kb_id, title)
    if not kb_id or not title or title == "" then return nil end
    local db = get_db()
    local rows, err = sql.builder.select("*")
        :from("keeper_kb_nodes")
        :where("kb_id = ?", kb_id)
        :where("title = ?", title)
        :limit(1)
        :run_with(db)
        :query()
    if err then return nil, "Failed to query by title: " .. err end
    if not rows or #rows == 0 then return nil end
    return row_to_node(rows[1])
end

function M.create(params)
    local kb_id = params.kb_id or consts.DEFAULT_KB_ID
    local db = get_db()
    local id = uuid.v4()
    local ts = now_iso()
    local refs_json = json.encode(params.refs or {})
    local meta_json = json.encode(params.metadata or {})
    local summary = params.summary or ""

    local _, err = sql.builder.insert("keeper_kb_nodes")
        :set_map({
            id = id,
            kb_id = kb_id,
            parent_id = nil_to_null(params.parent_id),
            workspace_id = nil_to_null(params.workspace_id),
            node_type = params.node_type or M.TYPE.PATTERN,
            title = params.title or "",
            summary = summary,
            content = params.content or "",
            source = params.source or M.SOURCE.HUMAN,
            confidence = params.confidence or 1.0,
            embedded = 0,
            scope_namespace = nil_to_null(params.scope_namespace),
            scope_kind = nil_to_null(params.scope_kind),
            scope_meta_type = nil_to_null(params.scope_meta_type),
            refs = refs_json,
            metadata = meta_json,
            created_at = ts,
            updated_at = ts,
        })
        :run_with(db)
        :exec()
    if err then return nil, "Failed to create node: " .. err end

    sql.builder.insert("keeper_kb_nodes_fts")
        :set_map({
            id = id,
            title = params.title or "",
            content = params.content or "",
            node_type = params.node_type or M.TYPE.PATTERN,
        })
        :run_with(db)
        :exec()

    local node = {
        id = id,
        kb_id = kb_id,
        node_type = params.node_type or M.TYPE.PATTERN,
        title = params.title or "",
        content = params.content or "",
        source = params.source or M.SOURCE.HUMAN,
        workspace_id = params.workspace_id,
        created_at = ts,
    }

    publish(consts.EVENTS.NODE_CREATED, node)
    return node
end

function M.update(id, params)
    local db = get_db()
    local ts = now_iso()
    local values = {}

    if params.kb_id ~= nil then values.kb_id = params.kb_id end
    if params.title ~= nil then values.title = params.title end
    if params.summary ~= nil then values.summary = params.summary end
    if params.content ~= nil then values.content = params.content; values.embedded = 0 end
    if params.node_type ~= nil then values.node_type = params.node_type end
    if params.parent_id ~= nil then values.parent_id = params.parent_id end
    if params.workspace_id ~= nil then values.workspace_id = params.workspace_id end
    if params.source ~= nil then values.source = params.source end
    if params.confidence ~= nil then values.confidence = params.confidence end
    if params.scope_namespace ~= nil then values.scope_namespace = params.scope_namespace end
    if params.scope_kind ~= nil then values.scope_kind = params.scope_kind end
    if params.scope_meta_type ~= nil then values.scope_meta_type = params.scope_meta_type end
    if params.refs ~= nil then values.refs = json.encode(params.refs) end
    if params.metadata ~= nil then values.metadata = json.encode(params.metadata) end

    if next(values) == nil then return nil, "No fields to update" end

    values.updated_at = ts
    local _, err = sql.builder.update("keeper_kb_nodes")
        :set_map(values)
        :where("id = ?", id)
        :run_with(db)
        :exec()
    if err then return nil, "Failed to update node: " .. err end

    sql.builder.delete("keeper_kb_nodes_fts")
        :where("id = ?", id)
        :run_with(db)
        :exec()
    local node = M.get(id)
    if node then
        sql.builder.insert("keeper_kb_nodes_fts")
            :set_map({ id = id, title = node.title, content = node.content, node_type = node.node_type })
            :run_with(db)
            :exec()
        publish(consts.EVENTS.NODE_UPDATED, node)
    end
    return { id = id, updated_at = ts }
end

function M.delete(id)
    local db = get_db()
    local node = M.get(id)
    sql.builder.update("keeper_kb_nodes")
        :set("parent_id", sql.NULL)
        :where("parent_id = ?", id)
        :run_with(db)
        :exec()
    sql.builder.delete("keeper_kb_nodes_fts"):where("id = ?", id):run_with(db):exec()
    sql.builder.delete("keeper_kb_embeddings"):where("node_id = ?", id):run_with(db):exec()
    local _, err = sql.builder.delete("keeper_kb_nodes")
        :where("id = ?", id)
        :run_with(db)
        :exec()
    if err then return nil, "Failed to delete node: " .. err end
    if node then publish(consts.EVENTS.NODE_DELETED, { id = id, title = node.title }) end
    return { id = id, deleted = true }
end

function M.delete_by_workspace(workspace_id)
    local db = get_db()
    local rows = sql.builder.select("id")
        :from("keeper_kb_nodes")
        :where("workspace_id = ?", workspace_id)
        :run_with(db)
        :query()
    local count = 0
    for _, row in ipairs(rows or {}) do
        M.delete(row.id)
        count = count + 1
    end
    return { workspace_id = workspace_id, deleted = count }
end

function M.get(id)
    local db = get_db()
    local rows, err = sql.builder.select("*")
        :from("keeper_kb_nodes")
        :where("id = ?", id)
        :run_with(db)
        :query()
    if err or not rows or #rows == 0 then return nil end
    return row_to_node(rows[1])
end

function M.list(params)
    local db = get_db()
    params = params or {}
    local q = sql.builder.select("*"):from("keeper_kb_nodes")

    if params.kb_id then q = q:where("kb_id = ?", params.kb_id) end
    if params.node_type then q = q:where("node_type = ?", params.node_type) end
    if params.source then q = q:where("source = ?", params.source) end
    if params.parent_id then q = q:where("parent_id = ?", params.parent_id) end
    if params.workspace_id then q = q:where("workspace_id = ?", params.workspace_id) end
    if params.embedded ~= nil then q = q:where("embedded = ?", params.embedded and 1 or 0) end
    if params.scope_namespace then q = q:where("scope_namespace = ?", params.scope_namespace) end
    if params.scope_kind then q = q:where("scope_kind = ?", params.scope_kind) end
    if params.scope_meta_type then q = q:where("scope_meta_type = ?", params.scope_meta_type) end

    local rows, err = q:order_by("updated_at DESC")
        :limit(params.limit or 200)
        :run_with(db)
        :query()
    if err then return nil, "Failed to list nodes: " .. err end

    local nodes = {}
    for _, row in ipairs(rows or {}) do table.insert(nodes, row_to_node(row)) end
    return nodes
end

function M.search_text(query, params)
    local db = get_db()
    params = params or {}
    local limit = params.limit or 20

    local rows, err
    if not is_postgres(db) then
        local kb_filter = ""
        local args = { query, limit }
        if params.kb_id then
            kb_filter = " AND n.kb_id = ?"
            args = { query, params.kb_id, limit }
        end

        rows, err = db:query([[
            SELECT n.* FROM keeper_kb_nodes n INNER JOIN keeper_kb_nodes_fts f ON f.id = n.id
            WHERE keeper_kb_nodes_fts MATCH ?]] .. kb_filter .. [[ ORDER BY rank LIMIT ?
        ]], args)
    end

    if not rows or err then
        local like_query = "%" .. query .. "%"
        local q = sql.builder.select("*"):from("keeper_kb_nodes")
        if is_postgres(db) then
            q = q:where("(title ILIKE ? OR content ILIKE ?)", like_query, like_query)
        else
            q = q:where("(title LIKE ? OR content LIKE ?)", like_query, like_query)
        end
        if params.kb_id then q = q:where("kb_id = ?", params.kb_id) end
        rows, err = q:order_by("updated_at DESC")
            :limit(limit)
            :run_with(db)
            :query()
        if err then return nil, "Search failed: " .. err end
    end

    local nodes = {}
    for _, row in ipairs(rows or {}) do table.insert(nodes, row_to_node(row)) end
    return nodes
end

function M.search_by_embedding(query_vec, params)
    local db = get_db()
    params = params or {}
    local limit = params.limit or 10

    local rows, err
    if not is_postgres(db) then
        rows, err = db:query([[
            SELECT node_id, distance FROM keeper_kb_embeddings
            WHERE embedding MATCH ? AND k = ?
        ]], { json.encode(query_vec), limit })
    end

    if err or is_postgres(db) or #(rows or {}) == 0 then
        local scan = sql.builder.select("e.node_id", "e.embedding")
            :from("keeper_kb_embeddings e")
            :join("keeper_kb_nodes n ON n.id = e.node_id")
        if params.kb_id then scan = scan:where("n.kb_id = ?", params.kb_id) end
        local emb_rows, scan_err = scan:run_with(db):query()
        if scan_err then return nil, "Semantic search failed: " .. scan_err end

        rows = {}
        for _, row in ipairs(emb_rows or {}) do
            local distance = cosine_distance(query_vec, tostring(row.embedding or ""))
            if distance then
                table.insert(rows, { node_id = row.node_id, distance = distance })
            end
        end
        table.sort(rows, compare_distance)
        while #rows > limit do table.remove(rows) end
    elseif not rows then
        return nil, "Semantic search failed"
    end

    local results = {}
    for _, row in ipairs(rows or {}) do
        local node = M.get(row.node_id)
        if node then
            if not params.kb_id or node.kb_id == params.kb_id then
                node.distance = row.distance
                table.insert(results, node)
            end
        end
    end
    return results
end

function M.stats(params)
    local db = get_db()
    params = params or {}

    local type_q = sql.builder.select("node_type", "COUNT(*) as count")
        :from("keeper_kb_nodes")
        :group_by("node_type")
    if params.kb_id then type_q = type_q:where("kb_id = ?", params.kb_id) end
    local rows, err = type_q:run_with(db):query()
    if err then return nil, err end

    local stats = { total = 0, by_type = {}, by_source = {} }
    for _, row in ipairs(rows or {}) do
        stats.by_type[row.node_type] = row.count
        stats.total = stats.total + row.count
    end

    local src_q = sql.builder.select("source", "COUNT(*) as count")
        :from("keeper_kb_nodes")
        :group_by("source")
    if params.kb_id then src_q = src_q:where("kb_id = ?", params.kb_id) end
    local src_rows = src_q:run_with(db):query()
    for _, row in ipairs(src_rows or {}) do stats.by_source[row.source] = row.count end

    local emb_q = sql.builder.select("COUNT(*) as count")
        :from("keeper_kb_nodes")
        :where("embedded = ?", 1)
    if params.kb_id then emb_q = emb_q:where("kb_id = ?", params.kb_id) end
    local emb_rows = emb_q:run_with(db):query()
    stats.embedded = (emb_rows and #emb_rows > 0) and emb_rows[1].count or 0

    local ws_q = sql.builder.select("COUNT(DISTINCT workspace_id) as count")
        :from("keeper_kb_nodes")
        :where("workspace_id IS NOT NULL")
    if params.kb_id then ws_q = ws_q:where("kb_id = ?", params.kb_id) end
    local ws_rows = ws_q:run_with(db):query()
    stats.workspace_linked = (ws_rows and #ws_rows > 0) and ws_rows[1].count or 0

    return stats
end

return M

