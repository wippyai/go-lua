local sql = require("sql")
local json = require("json")
local time = require("time")
local consts = require("consts")
local security = require("security")

type Artifact = {
    artifact_id: string,
    session_id: string?,
    user_id: string,
    kind: string,
    title: string,
    content: string?,
    meta: {[string]: any}?,
    created_at: string,
    updated_at: string,
}

type ArtifactUpdates = {
    kind: string?,
    title: string?,
    content: string?,
    meta: {[string]: any}?,
}

local artifact_repo = {}

-- Get a database connection
local function get_db()
    local DB_RESOURCE, _ = consts.get_db_resource()

    local db, err = sql.get(DB_RESOURCE)
    if err then
        return nil, "Failed to connect to database: " .. err
    end
    return db
end

-- Create a new artifact
function artifact_repo.create(artifact_id, session_id, kind, title, content, meta)
    session_id = session_id or nil

    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end
    if not kind or kind == "" then
        return nil, "Artifact kind is required"
    end

    local actor = security.actor()
    if not actor then
        return nil, "No authenticated user found"
    end

    local user_id

    local meta_json = nil
    if meta then
        if type(meta) == "table" then
            local encoded, err = json.encode(meta)
            if err then
                return nil, "Failed to encode meta: " .. err
            end
            meta_json = encoded
        else
            meta_json = meta
        end
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    if session_id and session_id ~= "" then
        local check_query = sql.builder.select("session_id", "user_id")
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

        user_id = sessions[1].user_id
    else
        user_id = actor:id()
    end

    local now = time.now():format(time.RFC3339)

    local insert_query = sql.builder.insert("artifacts")
        :set_map({
            artifact_id = artifact_id,
            session_id = session_id,
            user_id = user_id,
            kind = kind,
            title = title or "",
            content = content or "",
            meta = meta_json or sql.as.null(),
            created_at = now,
            updated_at = now
        })

    local insert_executor = insert_query:run_with(db)
    local result, err = insert_executor:exec()

    db:release()

    if err then
        return nil, "Failed to create artifact: " .. err
    end

    return {
        artifact_id = artifact_id,
        session_id = session_id,
        user_id = user_id,
        kind = kind,
        title = title,
        created_at = now,
        updated_at = now
    }
end

-- Get an artifact by ID
function artifact_repo.get(artifact_id)
    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("artifact_id", "session_id", "user_id", "kind", "title", "content", "meta", "created_at", "updated_at", "content_version")
        :from("artifacts")
        :where("artifact_id = ?", artifact_id)
        :limit(1)

    -- Execute the query
    local executor = query:run_with(db)
    local artifacts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to get artifact: " .. err
    end

    if #artifacts == 0 then
        return nil, "Artifact not found"
    end

    local artifact = artifacts[1]

    -- Parse meta JSON if it exists
    if artifact.meta and artifact.meta ~= "" then
        local decoded, err = json.decode(artifact.meta :: string)
        if not err then
            artifact.meta = decoded
        end
    end

    return artifact
end

-- Update artifact
function artifact_repo.update(artifact_id, updates)
    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end

    if not updates or type(updates) ~= "table" then
        return nil, "Updates must be a table"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if artifact exists
    local check_query = sql.builder.select("artifact_id")
        :from("artifacts")
        :where("artifact_id = ?", artifact_id)

    local check_executor = check_query:run_with(db)
    local artifacts, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if artifact exists: " .. err
    end

    if #artifacts == 0 then
        db:release()
        return nil, "Artifact not found"
    end

    -- Convert meta to JSON if it's a table
    local meta_value = updates.meta
    if meta_value and type(meta_value) == "table" then
        local encoded, err = json.encode(meta_value)
        if err then
            db:release()
            return nil, "Failed to encode meta: " .. err
        end
        meta_value = encoded
    end

    -- Build the update query
    local update_query = sql.builder.update("artifacts")

    -- Add fields to update
    local updated = false

    if updates.kind ~= nil then
        update_query = update_query:set("kind", updates.kind)
        updated = true
    end

    if updates.title ~= nil then
        update_query = update_query:set("title", updates.title)
        updated = true
    end

    if updates.content ~= nil then
        update_query = update_query:set("content", updates.content)
        -- Bumped here as well as in update_content, because this is the path the session
        -- flow actually uses: control_handlers -> writer:update_artifact -> update().
        -- Bumped only when the content changes; a title or meta edit is not a new version
        -- of the content and must not invalidate a cached body.
        update_query = update_query:set("content_version", sql.builder.expr("content_version + 1"))
        updated = true
    end

    if meta_value ~= nil then
        update_query = update_query:set("meta", meta_value)
        updated = true
    end

    -- Always update the updated_at timestamp
    update_query = update_query:set("updated_at", time.now():format(time.RFC3339))

    -- Add where clause
    update_query = update_query:where("artifact_id = ?", artifact_id)

    -- Execute the query
    local update_executor = update_query:run_with(db)
    local result, err = update_executor:exec()

    db:release()

    if err then
        return nil, "Failed to update artifact: " .. err
    end

    return {
        artifact_id = artifact_id,
        updated = true
    }
end

-- List artifacts by session ID
function artifact_repo.list_by_session(session_id, limit, offset)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("artifact_id", "session_id", "user_id", "kind", "title", "meta", "created_at", "updated_at")
        :from("artifacts")
        :where("session_id = ?", session_id)
        :order_by("created_at DESC")

    -- Add limit and offset if provided
    if limit and limit > 0 then
        query = query:limit(limit)
        if offset and offset > 0 then
            query = query:offset(offset)
        end
    end

    -- Execute the query
    local executor = query:run_with(db)
    local artifacts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to list artifacts: " .. err
    end

    -- Parse meta JSON if it exists
    for i, artifact in ipairs(artifacts) do
        if artifact.meta and artifact.meta ~= "" then
            local decoded, err = json.decode(artifact.meta :: string)
            if not err then
                artifact.meta = decoded
            end
        end
    end

    return artifacts
end

-- List metadata-only artifacts owned by a user. The optional session filter is
-- applied inside the same actor-scoped query. Cursor pagination uses the stable
-- (created_at, artifact_id) ordering so concurrent inserts cannot shift rows
-- between pages. Cursors carry only an opaque artifact id; the timestamp is
-- resolved inside the same actor/session scope instead of trusting client data.
function artifact_repo.list_by_user(user_id, session_id, limit, cursor_artifact_id)
    if not user_id or user_id == "" then
        return nil, "User ID is required"
    end

    limit = limit or 50
    if limit < 1 then limit = 1 end

    local db, err = get_db()
    if err then return nil, err end

    local cursor_created_at = nil
    if cursor_artifact_id then
        local cursor_filters = {
            sql.builder.expr("user_id = ?", user_id),
            sql.builder.expr("artifact_id = ?", cursor_artifact_id),
        }
        if session_id and session_id ~= "" then
            cursor_filters[#cursor_filters + 1] = sql.builder.expr("session_id = ?", session_id)
        end
        local cursor_query = sql.builder.select("created_at", "artifact_id")
            :from("artifacts")
            :where(sql.builder.and_(cursor_filters))
            :limit(1)
        local cursor_rows, cursor_err = cursor_query:run_with(db):query()
        if cursor_err then
            db:release()
            return nil, "Failed to resolve artifact cursor: " .. cursor_err
        end
        if #cursor_rows == 0 then
            db:release()
            return nil, "Invalid artifact cursor"
        end
        cursor_created_at = cursor_rows[1].created_at
    end

    local filters = { sql.builder.expr("user_id = ?", user_id) }
    if session_id and session_id ~= "" then
        filters[#filters + 1] = sql.builder.expr("session_id = ?", session_id)
    end
    if cursor_created_at and cursor_artifact_id then
        filters[#filters + 1] = sql.builder.expr(
            "(created_at < ? OR (created_at = ? AND artifact_id < ?))",
            cursor_created_at,
            cursor_created_at,
            cursor_artifact_id
        )
    end

    local query = sql.builder.select(
            "artifact_id", "session_id", "kind", "title", "meta", "created_at", "updated_at"
        )
        :from("artifacts")
        :where(sql.builder.and_(filters))
        :order_by("created_at DESC, artifact_id DESC")
        :limit(limit + 1)

    local artifacts, query_err = query:run_with(db):query()
    db:release()
    if query_err then return nil, "Failed to list user artifacts: " .. query_err end

    local has_more = #artifacts > limit
    if has_more then table.remove(artifacts) end
    for _, artifact in ipairs(artifacts) do
        if artifact.meta and artifact.meta ~= "" then
            local decoded, decode_err = json.decode(artifact.meta :: string)
            artifact.meta = decode_err and {} or decoded
        elseif not artifact.meta then
            artifact.meta = {}
        end
    end

    return { artifacts = artifacts, has_more = has_more }
end

-- List artifacts by kind
function artifact_repo.list_by_kind(session_id, kind, limit, offset)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    if not kind or kind == "" then
        return nil, "Artifact kind is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("artifact_id", "session_id", "user_id", "kind", "title", "meta", "created_at", "updated_at")
        :from("artifacts")
        :where(sql.builder.and_({
            sql.builder.expr("session_id = ?", session_id),
            sql.builder.expr("kind = ?", kind)
        }))
        :order_by("created_at DESC")

    -- Add limit and offset if provided
    if limit and limit > 0 then
        query = query:limit(limit)
        if offset and offset > 0 then
            query = query:offset(offset)
        end
    end

    -- Execute the query
    local executor = query:run_with(db)
    local artifacts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to list artifacts by kind: " .. err
    end

    -- Parse meta JSON if it exists
    for i, artifact in ipairs(artifacts) do
        if artifact.meta and artifact.meta ~= "" then
            local decoded, err = json.decode(artifact.meta :: string)
            if not err then
                artifact.meta = decoded
            end
        end
    end

    return artifacts
end

-- Get artifact content
function artifact_repo.get_content(artifact_id)
    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("content")
        :from("artifacts")
        :where("artifact_id = ?", artifact_id)
        :limit(1)

    -- Execute the query
    local executor = query:run_with(db)
    local results, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to get artifact content: " .. err
    end

    if #results == 0 then
        return nil, "Artifact not found"
    end

    return results[1].content
end

-- Update artifact content
function artifact_repo.update_content(artifact_id, content)
    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end

    if content == nil then
        return nil, "Content is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if artifact exists
    local check_query = sql.builder.select("artifact_id")
        :from("artifacts")
        :where("artifact_id = ?", artifact_id)

    local check_executor = check_query:run_with(db)
    local artifacts, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if artifact exists: " .. err
    end

    if #artifacts == 0 then
        db:release()
        return nil, "Artifact not found"
    end

    -- Build the UPDATE query
    -- content_version is bumped inside the same UPDATE rather than read, incremented and
    -- written back: two concurrent writers doing read-then-write would both observe the
    -- same value and one bump would be lost.
    local update_query = sql.builder.update("artifacts")
        :set("content", content)
        :set("updated_at", time.now():format(time.RFC3339))
        :set("content_version", sql.builder.expr("content_version + 1"))
        :where("artifact_id = ?", artifact_id)

    -- Execute the query
    local update_executor = update_query:run_with(db)
    local result, err = update_executor:exec()

    db:release()

    if err then
        return nil, "Failed to update artifact content: " .. err
    end

    -- Deliberately not returning the new content_version. Reading it back here would be a
    -- second statement on a fresh connection, so a concurrent writer could bump between the
    -- UPDATE and the read and this call would report a version it did not produce. It would
    -- also re-read the whole row including the content BLOB just to obtain an integer, which
    -- is why list_by_session omits that column too. Callers that need the value read it from
    -- the artifact endpoint, which is authoritative.
    return {
        artifact_id = artifact_id,
        updated = true
    }
end

-- Delete an artifact
function artifact_repo.delete(artifact_id)
    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if artifact exists
    local check_query = sql.builder.select("artifact_id")
        :from("artifacts")
        :where("artifact_id = ?", artifact_id)

    local check_executor = check_query:run_with(db)
    local artifacts, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if artifact exists: " .. err
    end

    if #artifacts == 0 then
        db:release()
        return nil, "Artifact not found"
    end

    -- Build the DELETE query
    local delete_query = sql.builder.delete("artifacts")
        :where("artifact_id = ?", artifact_id)

    -- Execute the query
    local delete_executor = delete_query:run_with(db)
    local result, err = delete_executor:exec()

    db:release()

    if err then
        return nil, "Failed to delete artifact: " .. err
    end

    return { deleted = true }
end

-- Count artifacts in a session
function artifact_repo.count_by_session(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select("COUNT(*) as count")
        :from("artifacts")
        :where("session_id = ?", session_id)

    -- Execute the query
    local executor = query:run_with(db)
    local result, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to count artifacts: " .. err
    end

    return result[1].count
end

-- Count artifacts by kind in a session
function artifact_repo.count_by_kind(session_id, kind)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    if not kind or kind == "" then
        return nil, "Artifact kind is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select("COUNT(*) as count")
        :from("artifacts")
        :where(sql.builder.and_({
            sql.builder.expr("session_id = ?", session_id),
            sql.builder.expr("kind = ?", kind)
        }))

    -- Execute the query
    local executor = query:run_with(db)
    local result, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to count artifacts by kind: " .. err
    end

    return result[1].count
end

return artifact_repo

