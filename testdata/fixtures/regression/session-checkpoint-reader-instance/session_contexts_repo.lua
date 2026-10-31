local sql = require("sql")
local json = require("json")
local time = require("time")
local consts = require("consts")

type SessionContext = {
    id: string,
    session_id: string,
    type: string,
    text: string,
    time: string,
}

local session_contexts_repo = {}

-- Get a database connection
local function get_db()
    local db_resource, _ = consts.get_db_resource()

    local db, err = sql.get(db_resource)
    if err then
        return nil, "Failed to connect to database: " .. err
    end
    return db
end

-- Create a new session context
function session_contexts_repo.create(id, session_id, context_type, text, pTime)
    if not id or id == "" then
        return nil, "ID is required"
    end

    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    if not context_type or context_type == "" then
        return nil, "Context type is required"
    end

    if not text then
        return nil, "Text is required"
    end

    -- Default time to current time if not provided
    if pTime == nil then
        pTime = time.now():utc():format(time.RFC3339)
    else
        pTime = time.unix(pTime, 0):format(time.RFC3339)
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

    -- Build the INSERT query
    local query = sql.builder.insert("session_contexts")
        :set_map({
            id = id,
            session_id = session_id,
            type = context_type,
            text = text,
            time = pTime
        })

    -- Execute the query
    local executor = query:run_with(db)
    local result, err = executor:exec()

    db:release()

    if err then
        return nil, "Failed to create session context: " .. err
    end

    return {
        id = id,
        session_id = session_id,
        type = context_type,
        text = text,
        time = pTime
    }
end

-- Get a session context by ID
function session_contexts_repo.get(id)
    if not id or id == "" then
        return nil, "ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("id", "session_id", "type", "text", "time")
        :from("session_contexts")
        :where("id = ?", id)
        :limit(1)

    -- Execute the query
    local executor = query:run_with(db)
    local contexts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to get session context: " .. err
    end

    if #contexts == 0 then
        return nil, "Session context not found"
    end

    return contexts[1]
end

-- List session contexts by session ID, sorted by ID for consistent order
function session_contexts_repo.list_by_session(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("id", "session_id", "type", "text", "time")
        :from("session_contexts")
        :where("session_id = ?", session_id)
        :order_by("id ASC") -- Order by ID for consistent order (UUID v7 is time-ordered)

    -- Execute the query
    local executor = query:run_with(db)
    local contexts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to list session contexts: " .. err
    end

    return contexts
end

-- List session contexts by type
function session_contexts_repo.list_by_type(session_id, context_type)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    if not context_type or context_type == "" then
        return nil, "Context type is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("id", "session_id", "type", "text", "time")
        :from("session_contexts")
        :where(sql.builder.and_({
            sql.builder.expr("session_id = ?", session_id),
            sql.builder.expr("type = ?", context_type)
        }))
        :order_by("id ASC") -- Order by ID for consistent order

    -- Execute the query
    local executor = query:run_with(db)
    local contexts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to list session contexts by type: " .. err
    end

    return contexts
end

-- Update session context text
function session_contexts_repo.update_text(id, text)
    if not id or id == "" then
        return nil, "ID is required"
    end

    if not text then
        return nil, "Text is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if context exists
    local check_query = sql.builder.select("id")
        :from("session_contexts")
        :where("id = ?", id)

    local check_executor = check_query:run_with(db)
    local contexts, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if context exists: " .. err
    end

    if #contexts == 0 then
        db:release()
        return nil, "Session context not found"
    end

    -- Build the UPDATE query
    local update_query = sql.builder.update("session_contexts")
        :set("text", text)
        :where("id = ?", id)

    -- Execute the query
    local update_executor = update_query:run_with(db)
    local result, err = update_executor:exec()

    db:release()

    if err then
        return nil, "Failed to update session context text: " .. err
    end

    return {
        id = id,
        text = text,
        updated = true
    }
end

-- Delete a session context
function session_contexts_repo.delete(id)
    if not id or id == "" then
        return nil, "ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if context exists
    local check_query = sql.builder.select("id")
        :from("session_contexts")
        :where("id = ?", id)

    local check_executor = check_query:run_with(db)
    local contexts, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if context exists: " .. err
    end

    if #contexts == 0 then
        db:release()
        return nil, "Session context not found"
    end

    -- Build the DELETE query
    local delete_query = sql.builder.delete("session_contexts")
        :where("id = ?", id)

    -- Execute the query
    local delete_executor = delete_query:run_with(db)
    local result, err = delete_executor:exec()

    db:release()

    if err then
        return nil, "Failed to delete session context: " .. err
    end

    return { deleted = true }
end

-- Delete all contexts for a session
function session_contexts_repo.delete_by_session(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the DELETE query
    local delete_query = sql.builder.delete("session_contexts")
        :where("session_id = ?", session_id)

    -- Execute the query
    local delete_executor = delete_query:run_with(db)
    local result, err = delete_executor:exec()

    db:release()

    if err then
        return nil, "Failed to delete session contexts: " .. err
    end

    return {
        deleted = true,
        count = result.rows_affected
    }
end

-- Count session contexts for a session
function session_contexts_repo.count_by_session(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select("COUNT(*) as count")
        :from("session_contexts")
        :where("session_id = ?", session_id)

    -- Execute the query
    local executor = query:run_with(db)
    local result, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to count session contexts: " .. err
    end

    return result[1].count
end

-- Count session contexts by type for a session
function session_contexts_repo.count_by_type(session_id, context_type)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    if not context_type or context_type == "" then
        return nil, "Context type is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select("COUNT(*) as count")
        :from("session_contexts")
        :where(sql.builder.and_({
            sql.builder.expr("session_id = ?", session_id),
            sql.builder.expr("type = ?", context_type)
        }))

    local executor = query:run_with(db)
    local result, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to count session contexts by type: " .. err
    end

    return result[1].count
end

return session_contexts_repo

