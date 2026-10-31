local sql = require("sql")
local uuid = require("uuid")
local time = require("time")

-- The only durable engine state: the mapping (provider, external_id) ->
-- session_id. The subject/identity is resolved per turn, the agent + history live
-- in the session; none of those are stored here. Parameterized by (db_id, table)
-- so a DM and a channel mapping share one implementation over their own tables.
-- get / get_or_create / set / clear is the whole surface. clear rotates to a
-- fresh session_id so the next turn starts a new chat, while a reused id
-- rehydrates.
local M = {}

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function now(): string
    return time.now():utc():format_rfc3339()
end

local function valid_table_name(value: any): boolean
    return type(value) == "string" and value:match("^[a-z_][a-z0-9_]*$") ~= nil
end

local function open_db(db_id: string): (sql.DB?, string?)
    if trim(db_id) == "" then return nil, "db_id is required" end
    local db, err = sql.get(db_id)
    if err or not db then return nil, "session db unavailable: " .. tostring(err) end
    return db :: sql.DB, nil
end

local function set_session(db_id: string, table_name: string, provider: string, external_id: string, session_id: string): string?
    if not valid_table_name(table_name) then return "invalid session table name" end
    local db, db_err = open_db(db_id)
    if not db then return db_err end
    local _, xerr = db:execute(
        "INSERT INTO " .. table_name .. " (provider, external_id, session_id, updated_at)" ..
        " VALUES ($1, $2, $3, $4)" ..
        " ON CONFLICT (provider, external_id) DO UPDATE SET" ..
        " session_id = excluded.session_id, updated_at = excluded.updated_at",
        { provider, external_id, session_id, now() })
    db:release()
    if xerr then return "session upsert failed: " .. tostring(xerr) end
    return nil
end

local function stale_by(updated_at: any, max_idle_seconds: any): (boolean, string?)
    local cutoff = tonumber(max_idle_seconds) or 0
    if cutoff <= 0 then return false, nil end
    local raw = trim(updated_at)
    if raw == "" then return true, nil end
    local parsed, perr = time.parse(time.RFC3339, raw)
    if perr or not parsed then return false, "invalid session updated_at: " .. tostring(perr or raw) end
    local age = time.now():utc():sub(parsed):seconds()
    return age >= cutoff, nil
end

-- get(db_id, table, provider, external_id) -> (session_id | nil, err?). nil
-- session_id means no session has been mapped yet (a fresh conversation).
function M.get(db_id: string, table_name: string, provider: string, external_id: string): (string?, string?)
    if not valid_table_name(table_name) then return nil, "invalid session table name" end
    provider = trim(provider)
    external_id = trim(external_id)
    if provider == "" then return nil, "provider is required" end
    if external_id == "" then return nil, "external_id is required" end

    local db, db_err = open_db(db_id)
    if not db then return nil, db_err end
    local rows, qerr = db:query(
        "SELECT session_id FROM " .. table_name .. " WHERE provider = $1 AND external_id = $2 LIMIT 1",
        { provider, external_id })
    db:release()
    if qerr then return nil, tostring(qerr) end
    if not rows or not rows[1] then return nil, nil end
    local session_id = trim(rows[1].session_id)
    if session_id == "" then return nil, nil end
    return session_id, nil
end

-- get_or_create(...) -> (session_id, created, err). Returns the mapped session id,
-- or mints and persists a new one on first contact. max_idle_seconds rotates an
-- idle mapping before routing the next turn, then touches the surviving mapping.
function M.get_or_create(db_id: string, table_name: string, provider: string, external_id: string, max_idle_seconds: integer?): (string?, boolean, string?)
    if not valid_table_name(table_name) then return nil, false, "invalid session table name" end
    provider = trim(provider)
    external_id = trim(external_id)
    if provider == "" then return nil, false, "provider is required" end
    if external_id == "" then return nil, false, "external_id is required" end

    local db, db_err = open_db(db_id)
    if not db then return nil, false, db_err end
    local rows, qerr = db:query(
        "SELECT session_id, updated_at FROM " .. table_name .. " WHERE provider = $1 AND external_id = $2 LIMIT 1",
        { provider, external_id })
    db:release()
    if qerr then return nil, false, tostring(qerr) end
    if rows and rows[1] then
        local existing = trim(rows[1].session_id)
        local stale, serr = stale_by(rows[1].updated_at, max_idle_seconds)
        if serr then return nil, false, serr end
        if existing ~= "" and not stale then
            local touch_err = set_session(db_id, table_name, provider, external_id, existing)
            if touch_err then return nil, false, touch_err end
            return existing, false, nil
        end
    end

    local session_id = uuid.v7()
    local serr = set_session(db_id, table_name, provider, external_id, session_id)
    if serr then return nil, false, serr end
    return session_id, true, nil
end

-- set(...) -> err?. Pins a specific session id for the mapping.
function M.set(db_id: string, table_name: string, provider: string, external_id: string, session_id: string): string?
    provider = trim(provider)
    external_id = trim(external_id)
    session_id = trim(session_id)
    if provider == "" then return "provider is required" end
    if external_id == "" then return "external_id is required" end
    if session_id == "" then return "session_id is required" end
    return set_session(db_id, table_name, provider, external_id, session_id)
end

-- remove(...) -> err?. Deletes the mapping row entirely. Used when the
-- conversation's owner is torn down (responder uninstalled, link/user/connection
-- deleted): clear rotates to a fresh session, this drops the mapping so nothing
-- points at the now-deleted session. Removing an absent row is not an error.
function M.remove(db_id: string, table_name: string, provider: string, external_id: string): string?
    if not valid_table_name(table_name) then return "invalid session table name" end
    provider = trim(provider)
    external_id = trim(external_id)
    if provider == "" then return "provider is required" end
    if external_id == "" then return "external_id is required" end
    local db, db_err = open_db(db_id)
    if not db then return db_err end
    local _, xerr = db:execute(
        "DELETE FROM " .. table_name .. " WHERE provider = $1 AND external_id = $2",
        { provider, external_id })
    db:release()
    if xerr then return "session delete failed: " .. tostring(xerr) end
    return nil
end

-- clear(...) -> (new_session_id, err). Rotates the mapping to a fresh session id
-- so the next turn starts a new chat. A row is created if none exists.
function M.clear(db_id: string, table_name: string, provider: string, external_id: string): (string?, string?)
    provider = trim(provider)
    external_id = trim(external_id)
    if provider == "" then return nil, "provider is required" end
    if external_id == "" then return nil, "external_id is required" end
    local session_id = uuid.v7()
    local serr = set_session(db_id, table_name, provider, external_id, session_id)
    if serr then return nil, serr end
    return session_id, nil
end

return M

