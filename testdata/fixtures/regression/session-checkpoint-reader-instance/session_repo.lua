local sql = require("sql")
local json = require("json")
local time = require("time")
local consts = require("consts")

type Session = {
    session_id: string,
    user_id: string,
    status: string?,
    primary_context_id: string,
    title: string,
    kind: string,
    meta: {[string]: any},
    config: {[string]: any},
    public_meta: {[string]: any},
    start_date: string,
    last_message_date: string,
}

type SessionUpdates = {
    title: string?,
    status: string?,
    kind: string?,
    meta: {[string]: any}?,
    config: {[string]: any}?,
    public_meta: {[string]: any}?,
    last_message_date: number?,
}

local session_repo = {}

-- Get a database connection
local function get_db()
    local db_resource = consts.get_db_resource()
    local db, err = sql.get(db_resource)
    if err then
        return nil, "Failed to connect to database: " .. err
    end
    return db
end

-- Count sessions by user ID
function session_repo.count_by_user(user_id)
    if not user_id or user_id == "" then
        return nil, "User ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select("COUNT(*) as total")
        :from("sessions")
        :where("user_id = ?", user_id)

    local executor = query:run_with(db)
    local result, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to count sessions: " .. err
    end

    if #result == 0 then
        return 0, nil
    end

    return result[1].total, nil
end

-- Create a new session
function session_repo.create(session_id, user_id, primary_context_id, title, kind, meta, config)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    if not user_id or user_id == "" then
        return nil, "User ID is required"
    end

    if not primary_context_id or primary_context_id == "" then
        return nil, "Primary context ID is required"
    end

    -- Default values
    title = title or ""
    kind = kind or "default"
    meta = meta or {}
    config = config or {}

    -- Encode JSON fields
    local encoded_meta, err = json.encode(meta)
    if err then
        return nil, "Failed to encode meta: " .. err
    end

    local encoded_config, err = json.encode(config)
    if err then
        return nil, "Failed to encode config: " .. err
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local now = time.now():format(time.RFC3339)

    local query = sql.builder.insert("sessions")
        :set_map({
            session_id = session_id,
            user_id = user_id,
            primary_context_id = primary_context_id,
            title = title,
            kind = kind,
            meta = encoded_meta,
            config = encoded_config,
            public_meta = '{}',
            start_date = now,
            last_message_date = now
        })

    local executor = query:run_with(db)
    local result, err = executor:exec()

    db:release()

    if err then
        return nil, "Failed to create session: " .. err
    end

    return {
        session_id = session_id,
        user_id = user_id,
        primary_context_id = primary_context_id,
        title = title,
        kind = kind,
        meta = meta,
        config = config,
        public_meta = {},
        start_date = now,
        last_message_date = now
    }
end

-- Get a session by ID filtered by user_id
function session_repo.get(session_id, user_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select(
            "session_id", "user_id", "status", "primary_context_id",
            "title", "kind", "meta", "config", "public_meta", "start_date", "last_message_date"
        )
        :from("sessions")
        :where("session_id = ?", session_id)
        :limit(1)

    local executor = query:run_with(db)
    local sessions, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to get session: " .. err
    end

    if #sessions == 0 then
        return nil, "Session not found"
    end

    local session = sessions[1]

    if user_id and session.user_id ~= user_id then
        return nil, "Session not found"
    end

    -- Parse JSON fields
    if session.meta and session.meta ~= "" then
        local decoded, err = json.decode(session.meta :: string)
        if not err then
            session.meta = decoded
        else
            session.meta = {}
        end
    else
        session.meta = {}
    end

    if session.config and session.config ~= "" then
        local decoded, err = json.decode(session.config :: string)
        if not err then
            session.config = decoded
        else
            session.config = {}
        end
    else
        session.config = {}
    end

    if session.public_meta and session.public_meta ~= "" then
        local decoded, err = json.decode(session.public_meta :: string)
        if not err then
            session.public_meta = decoded
        else
            session.public_meta = {}
        end
    else
        session.public_meta = {}
    end

    return session
end

-- List sessions by user ID
function session_repo.list_by_user(user_id, limit, offset)
    if not user_id or user_id == "" then
        return nil, "User ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select(
            "session_id", "user_id", "status", "primary_context_id",
            "title", "kind", "meta", "config", "public_meta", "start_date", "last_message_date"
        )
        :from("sessions")
        :where("user_id = ?", user_id)
        :order_by("last_message_date DESC")

    if limit and limit > 0 then
        query = query:limit(limit)
        if offset and offset > 0 then
            query = query:offset(offset)
        end
    end

    local executor = query:run_with(db)
    local sessions, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to list sessions: " .. err
    end

    -- Parse JSON fields for each session
    for i, session in ipairs(sessions) do
        if session.meta and session.meta ~= "" then
            local decoded, err = json.decode(session.meta :: string)
            if not err then
                session.meta = decoded
            else
                session.meta = {}
            end
        else
            session.meta = {}
        end

        if session.config and session.config ~= "" then
            local decoded, err = json.decode(session.config :: string)
            if not err then
                session.config = decoded
            else
                session.config = {}
            end
        else
            session.config = {}
        end

        if session.public_meta and session.public_meta ~= "" then
            local decoded, err = json.decode(session.public_meta :: string)
            if not err then
                session.public_meta = decoded
            else
                session.public_meta = {}
            end
        else
            session.public_meta = {}
        end
    end

    return sessions
end

-- Update session metadata (title, meta, config, public_meta, status, last_message_date)
function session_repo.update_session_meta(session_id, updates)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    if not updates or type(updates) ~= "table" then
        return nil, "Updates table is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if session exists
    local check_query = sql.builder.select("session_id")
        :from("sessions")
        :where("session_id = ?", session_id)

    local check_executor = check_query:run_with(db)
    local sessions, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if session exists: " .. err
    end

    if #sessions == 0 then
        db:release()
        return nil, "Session not found"
    end

    -- Build update query
    local update_query = sql.builder.update("sessions")
    local result = { session_id = session_id, updated = true }

    -- Handle individual field updates
    if updates.title ~= nil then
        update_query = update_query:set("title", updates.title)
        result.title = updates.title
    end

    if updates.status ~= nil then
        update_query = update_query:set("status", updates.status)
        result.status = updates.status
    end

    if updates.kind ~= nil then
        update_query = update_query:set("kind", updates.kind)
        result.kind = updates.kind
    end

    if updates.meta ~= nil then
        local encoded_meta, err = json.encode(updates.meta)
        if err then
            db:release()
            return nil, "Failed to encode meta: " .. err
        end
        update_query = update_query:set("meta", encoded_meta)
        result.meta = updates.meta
    end

    if updates.config ~= nil then
        local encoded_config, err = json.encode(updates.config)
        if err then
            db:release()
            return nil, "Failed to encode config: " .. err
        end
        update_query = update_query:set("config", encoded_config)
        result.config = updates.config
    end

    if updates.public_meta ~= nil then
        local encoded_public_meta, err = json.encode(updates.public_meta)
        if err then
            db:release()
            return nil, "Failed to encode public_meta: " .. err
        end
        update_query = update_query:set("public_meta", encoded_public_meta)
        result.public_meta = updates.public_meta
    end

    -- Always update last_message_date if any field is updated
    local date
    if updates.last_message_date ~= nil then
        date = time.unix(updates.last_message_date, 0):format(time.RFC3339)
    else
        date = time.now():format(time.RFC3339)
    end
    update_query = update_query:set("last_message_date", date)
    result.last_message_date = date

    -- Add WHERE clause
    update_query = update_query:where("session_id = ?", session_id)

    -- Execute the query
    local update_executor = update_query:run_with(db)
    local update_result, err = update_executor:exec()

    db:release()

    if err then
        return nil, "Failed to update session metadata: " .. err
    end

    return result
end

-- Delete a session and all its relationships
function session_repo.delete(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Begin transaction
    local tx, err = db:begin()
    if err then
        db:release()
        return nil, "Failed to begin transaction: " .. err
    end

    -- Delete artifacts first
    local artifacts_delete_query = sql.builder.delete("artifacts")
        :where("session_id = ?", session_id)

    local artifacts_delete_executor = artifacts_delete_query:run_with(tx)
    local result, err = artifacts_delete_executor:exec()

    if err then
        tx:rollback()
        db:release()
        return nil, "Failed to delete session artifacts: " .. err
    end

    -- Delete session contexts
    local contexts_delete_query = sql.builder.delete("session_contexts")
        :where("session_id = ?", session_id)

    local contexts_delete_executor = contexts_delete_query:run_with(tx)
    result, err = contexts_delete_executor:exec()

    if err then
        tx:rollback()
        db:release()
        return nil, "Failed to delete session contexts: " .. err
    end

    -- Delete messages
    local msg_delete_query = sql.builder.delete("messages")
        :where("session_id = ?", session_id)

    local msg_delete_executor = msg_delete_query:run_with(tx)
    result, err = msg_delete_executor:exec()

    if err then
        tx:rollback()
        db:release()
        return nil, "Failed to delete session messages: " .. err
    end

    -- Delete the session
    local session_delete_query = sql.builder.delete("sessions")
        :where("session_id = ?", session_id)

    local session_delete_executor = session_delete_query:run_with(tx)
    result, err = session_delete_executor:exec()

    if err then
        tx:rollback()
        db:release()
        return nil, "Failed to delete session: " .. err
    end

    if result.rows_affected == 0 then
        tx:rollback()
        db:release()
        return nil, "Session not found"
    end

    -- Commit transaction
    local success, err = tx:commit()
    if err then
        tx:rollback()
        db:release()
        return nil, "Failed to commit transaction: " .. err
    end

    db:release()

    return { deleted = true }
end

return session_repo

