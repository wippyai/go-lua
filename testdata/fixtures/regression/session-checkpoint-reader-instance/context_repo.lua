local sql = require("sql")
local time = require("time")
local consts = require("consts")

type Context = {
    context_id: string,
    type: string,
    data: string,
}

local context_repo = {}

-- Get a database connection
local function get_db()
    local DB_RESOURCE, _ = consts.get_db_resource()

    local db, err = sql.get(DB_RESOURCE)
    if err then
        return nil, "Failed to connect to database: " .. err
    end
    return db
end

-- Create a new context
function context_repo.create(context_id, type, data)
    if not context_id or context_id == "" then
        return nil, "Context ID is required"
    end

    if not type or type == "" then
        return nil, "Context type is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the INSERT query
    local query = sql.builder.insert("contexts")

        :set_map({
        context_id = context_id,
        type = type,
        data = data
    })

    -- Execute the query
    local executor = query:run_with(db)
    local result, err = executor:exec()

    db:release()

    if err then
        return nil, "Failed to create context: " .. err
    end

    return {
        context_id = context_id,
        type = type
    }
end

-- Get a context by ID
function context_repo.get(context_id)
    if not context_id or context_id == "" then
        return nil, "Context ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query with proper parameterized query
    local query = sql.builder.select("context_id", "type", "data")
        :from("contexts")
        :where("context_id = ?", context_id)

    -- Execute the query
    local executor = query:run_with(db)
    local contexts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to get context: " .. err
    end

    if #contexts == 0 then
        return nil, "Context not found"
    end

    return contexts[1]
end

-- Update context data
function context_repo.update(context_id, data)
    if not context_id or context_id == "" then
        return nil, "Context ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if context exists
    local check_query = sql.builder.select("context_id")
        :from("contexts")
        :where("context_id = ?", context_id)

    local check_executor = check_query:run_with(db)
    local contexts, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if context exists: " .. err
    end

    if #contexts == 0 then
        db:release()
        return nil, "Context not found"
    end

    -- Build the UPDATE query
    local update_query = sql.builder.update("contexts")

        :set("data", data)
        :where("context_id = ?", context_id)

    -- Execute the query
    local update_executor = update_query:run_with(db)
    local result, err = update_executor:exec()

    db:release()

    if err then
        return nil, "Failed to update context: " .. err
    end

    return {
        context_id = context_id,
        updated = true
    }
end

-- Get contexts by type
function context_repo.get_by_type(type, limit, offset)
    if not type or type == "" then
        return nil, "Context type is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Build the SELECT query
    local query = sql.builder.select("context_id", "type", "data")
        :from("contexts")
        :where("type = ?", type)

    -- Add limit and offset if provided
    if limit and limit > 0 then
        query = query:limit(limit)

        if offset and offset > 0 then
            query = query:offset(offset)
        end
    end

    -- Execute the query
    local executor = query:run_with(db)
    local contexts, err = executor:query()

    db:release()

    if err then
        return nil, "Failed to get contexts by type: " .. err
    end

    return contexts
end

-- Delete a context
function context_repo.delete(context_id)
    if not context_id or context_id == "" then
        return nil, "Context ID is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    -- Check if context exists
    local check_query = sql.builder.select("context_id")
        :from("contexts")
        :where("context_id = ?", context_id)

    local check_executor = check_query:run_with(db)
    local contexts, err = check_executor:query()

    if err then
        db:release()
        return nil, "Failed to check if context exists: " .. err
    end

    if #contexts == 0 then
        db:release()
        return nil, "Context not found"
    end

    -- Build the DELETE query
    local delete_query = sql.builder.delete("contexts")
        :where("context_id = ?", context_id)

    -- Execute the query
    local delete_executor = delete_query:run_with(db)
    local result, err = delete_executor:exec()

    db:release()

    if err then
        return nil, "Failed to delete context: " .. err
    end

    return { deleted = true }
end

return context_repo

