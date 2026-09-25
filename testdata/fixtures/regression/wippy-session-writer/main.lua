local uuid = require("uuid")
local json = require("json")
local security = require("security")

type WriteResult = {
    success: boolean,
    error: string?,
}

local session_writer = {
    _session_repo = require("session_repo"),
    _message_repo = require("message_repo"),
    _artifact_repo = require("artifact_repo"),
    _context_repo = require("context_repo"),
    _session_contexts_repo = require("session_contexts_repo"),
    session_id = nil :: string?,
    user_id = nil :: string?,
    actor = nil :: any,
    _session_data = nil :: any,
}
session_writer.__index = session_writer

function session_writer.new(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local actor = security.actor()
    if not actor then
        return nil, "No security actor available - writer requires authenticated context"
    end

    local user_id = actor:id()
    if not user_id or user_id == "" then
        return nil, "Actor ID is required for writer"
    end

    local allowed = security.can("write", "session:" .. session_id)
    if not allowed then
        return nil, "Permission denied for session access"
    end

    local session_data, err = session_writer._session_repo.get(session_id, user_id)
    if err then
        return nil, "Failed to get session: " .. err
    end

    if not session_data then
        return nil, "Session not found"
    end

    local self = setmetatable({}, session_writer)
    self.session_id = session_id
    self.user_id = user_id
    self.actor = actor
    self._session_data = session_data
    return self, nil
end

-- SESSION OPERATIONS

function session_writer:update_meta(updates)
    if not updates or type(updates) ~= "table" then
        return nil, "Updates must be a table"
    end

    local result, err = session_writer._session_repo.update_session_meta(self.session_id, updates)
    if err then
        return nil, "Failed to update session metadata: " .. err
    end

    return true
end

function session_writer:update_title(title)
    if not title or title == "" then
        return nil, "Title cannot be empty"
    end

    return self:update_meta({ title = title })
end

function session_writer:update_status(status, error_message)
    local updates = { status = status, last_message_date = os.time() }

    if error_message then
        local session, err = session_writer._session_repo.get(self.session_id, self.user_id)
        if session then
            local current_meta = session.meta or {}
            current_meta.error = error_message
            updates.meta = current_meta
        end
    end

    return self:update_meta(updates)
end

-- MESSAGE OPERATIONS

function session_writer:add_message(msg_type, content, metadata)
    if not msg_type or msg_type == "" then
        return nil, "Message type is required"
    end

    if content == nil then
        return nil, "Message content is required"
    end

    metadata = metadata or {}

    local message_id
    if metadata.message_id then
        message_id = metadata.message_id
        local clean = {}
        for k, v in pairs(metadata) do
            if k ~= "message_id" then
                clean[k] = v
            end
        end
        metadata = clean
    else
        local err
        message_id, err = uuid.v7()
        if err then
            return nil, "Failed to generate message ID: " .. err
        end
    end

    local result, err = session_writer._message_repo.create(message_id, self.session_id, msg_type, content, metadata)
    if err then
        return nil, "Failed to create message: " .. err
    end

    return message_id
end

function session_writer:update_message_meta(message_id, metadata)
    if not message_id or message_id == "" then
        return nil, "Message ID is required"
    end

    if not metadata or type(metadata) ~= "table" then
        return nil, "Metadata must be a table"
    end

    local result, err = session_writer._message_repo.update_metadata(message_id, metadata)
    if err then
        return nil, "Failed to update message metadata: " .. err
    end

    return true
end

function session_writer:add_function_call(function_name, arguments, metadata)
    if not function_name or function_name == "" then
        return nil, "Function name is required"
    end

    metadata = metadata or {}
    metadata.function_name = function_name
    metadata.status = "pending"

    if type(arguments) == "table" then
        local encoded, err = json.encode(arguments)
        if err then
            return nil, "Failed to encode arguments: " .. err
        end
        arguments = encoded
    end

    return self:add_message("function", arguments, metadata)
end

function session_writer:update_function_result(message_id, result, success, additional_metadata)
    if not message_id or message_id == "" then
        return nil, "Message ID is required"
    end

    if result == nil then
        return nil, "Function result is required"
    end

    local message, err = session_writer._message_repo.get(message_id)
    if err then
        return nil, "Failed to get message: " .. err
    end

    if not message then
        return nil, "Message not found"
    end

    local metadata = message.metadata or {}
    metadata.result = result
    metadata.status = success and "success" or "error"

    if additional_metadata and type(additional_metadata) == "table" then
        for k, v in pairs(additional_metadata) do
            metadata[k] = v
        end
    end

    return self:update_message_meta(message_id, metadata)
end

-- ARTIFACT OPERATIONS

function session_writer:create_artifact(artifact_id, kind, title, content, meta)
    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end

    if not kind or kind == "" then
        return nil, "Artifact kind is required"
    end

    local result, err = session_writer._artifact_repo.create(artifact_id, self.session_id, kind, title, content, meta)
    if err then
        return nil, "Failed to create artifact: " .. err
    end

    return artifact_id
end

function session_writer:update_artifact(artifact_id, updates)
    if not artifact_id or artifact_id == "" then
        return nil, "Artifact ID is required"
    end

    if not updates or type(updates) ~= "table" then
        return nil, "Updates must be a table"
    end

    local artifact, err = session_writer._artifact_repo.get(artifact_id)
    if err then
        return nil, "Failed to get artifact: " .. err
    end

    if not artifact then
        return nil, "Artifact not found"
    end

    if artifact.session_id ~= self.session_id then
        return nil, "Artifact belongs to different session"
    end

    local result, err = session_writer._artifact_repo.update(artifact_id, updates)
    if err then
        return nil, "Failed to update artifact: " .. err
    end

    return true
end

-- PRIMARY CONTEXT OPERATIONS (JSON key-value storage)

function session_writer:set_context(key, value)
    if not key then
        return nil, "Context key is required"
    end

    local context, err = session_writer._context_repo.get(self._session_data.primary_context_id)
    if err then
        return nil, "Failed to get context: " .. err
    end

    local data = {}
    if context and context.data then
        if type(context.data) == "string" then
            local decoded, parse_err = json.decode(context.data)
            if not parse_err and type(decoded) == "table" then
                data = decoded
            end
        elseif type(context.data) == "table" then
            data = context.data
        end
    end

    data[key] = value

    local encoded_data, encode_err = json.encode(data)
    if encode_err then
        return nil, "Failed to encode context data: " .. encode_err
    end

    local result, err = session_writer._context_repo.update(self._session_data.primary_context_id, encoded_data)
    if err then
        return nil, "Failed to update context: " .. err
    end

    return true
end

function session_writer:delete_context(key)
    if not key then
        return nil, "Context key is required"
    end

    local context, err = session_writer._context_repo.get(self._session_data.primary_context_id)
    if err then
        return nil, "Failed to get context: " .. err
    end

    if not context or not context.data then
        return true
    end

    local data = {}
    if type(context.data) == "string" then
        local decoded, parse_err = json.decode(context.data)
        if not parse_err and type(decoded) == "table" then
            data = decoded
        end
    elseif type(context.data) == "table" then
        data = context.data
    end

    data[key] = nil

    local encoded_data, encode_err = json.encode(data)
    if encode_err then
        return nil, "Failed to encode context data: " .. encode_err
    end

    local result, err = session_writer._context_repo.update(self._session_data.primary_context_id, encoded_data)
    if err then
        return nil, "Failed to update context: " .. err
    end

    return true
end

-- SESSION CONTEXT OPERATIONS (session_contexts table - type + text)

function session_writer:add_session_context(context_type, text, timestamp)
    if not context_type or context_type == "" then
        return nil, "Context type is required"
    end

    if not text or text == "" then
        return nil, "Context text is required"
    end

    local context_id, err = uuid.v7()
    if err then
        return nil, "Failed to generate context ID: " .. err
    end

    local result, err = session_writer._session_contexts_repo.create(context_id, self.session_id, context_type, text, timestamp)
    if err then
        return nil, "Failed to create session context: " .. err
    end

    return context_id
end

function session_writer:delete_session_context(context_id)
    if not context_id or context_id == "" then
        return nil, "Context ID is required"
    end

    local context, err = session_writer._session_contexts_repo.get(context_id)
    if err then
        return nil, "Failed to get session context: " .. err
    end

    if not context then
        return nil, "Session context not found"
    end

    if context.session_id ~= self.session_id then
        return nil, "Context belongs to different session"
    end

    local result, err = session_writer._session_contexts_repo.delete(context_id)
    if err then
        return nil, "Failed to delete session context: " .. err
    end

    return true
end

function session_writer:delete_session_contexts_by_type(context_type)
    if not context_type or context_type == "" then
        return nil, "Context type is required"
    end

    local contexts, err = session_writer._session_contexts_repo.list_by_type(self.session_id, context_type)
    if err then
        return nil, "Failed to list session contexts: " .. err
    end

    local deleted_count = 0
    for _, context in ipairs(contexts) do
        local success, del_err = session_writer._session_contexts_repo.delete(context.id)
        if success then
            deleted_count = deleted_count + 1
        end
    end

    return { deleted_count = deleted_count }
end

return session_writer
