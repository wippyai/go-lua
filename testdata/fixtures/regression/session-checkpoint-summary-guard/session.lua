local security = require("security")
local json = require("json")
local consts = require("consts")

type SessionState = {
    session_id: string,
    user_id: string,
    status: string?,
    title: string,
    kind: string,
    meta: {[string]: any},
    config: {[string]: any},
    public_meta: {[string]: any},
    start_date: string,
    last_message_date: string,
    primary_context_id: string,
}

local session = {
    _session_repo = require("session_repo"),
    _message_repo = require("message_repo"),
    _artifact_repo = require("artifact_repo"),
    _session_contexts_repo = require("session_contexts_repo"),
    _context_repo = require("context_repo")
}

-- Query builders
local message_query = {
    _session_id = nil :: string?,
    _reader = nil :: any,
    _type_filter = nil :: string?,
    _limit = nil :: number?,
    _offset = nil :: number?,
    _after_message_id = nil :: string?,
    _error = nil :: string?,
}
message_query.__index = message_query

local artifact_query = {
    _session_id = nil :: string?,
    _kind_filter = nil :: string?,
    _limit = nil :: number?,
    _offset = nil :: number?,
    _error = nil :: string?,
}
artifact_query.__index = artifact_query

local context_query = {
    _session_id = nil :: string?,
    _type_filter = nil :: string?,
    _error = nil :: string?,
}
context_query.__index = context_query

-- Session reader
local session_reader = {
    session_id = nil :: string?,
    user_id = nil :: string?,
    actor = nil :: any,
    _session_data = nil :: any,
    _primary_context_cache = nil :: any,
}
session_reader.__index = session_reader

function session.open(session_id)
    if not session_id or session_id == "" then
        return nil, "Session ID is required"
    end

    local actor = security.actor()
    if not actor then
        return nil, "No security actor available - session requires authenticated context"
    end

    local user_id = actor:id()
    if not user_id or user_id == "" then
        return nil, "Actor ID is required"
    end

    local allowed = security.can("read", "session:" .. session_id)
    if not allowed then
        return nil, "Permission denied for session access"
    end

    local session_data, err = session._session_repo.get(session_id, user_id)
    if err then
        return nil, "Failed to get session: " .. err
    end

    if not session_data then
        return nil, "Session not found"
    end

    local self = setmetatable({}, session_reader)
    self.session_id = session_id
    self.user_id = user_id
    self.actor = actor
    self._session_data = session_data
    self._primary_context_cache = nil
    return self, nil
end

function session_reader:reset()
    local session_data, err = session._session_repo.get(self.session_id, self.user_id)
    if err then
        return nil, "Failed to reload session data: " .. err
    end

    if not session_data then
        return nil, "Session not found during reset"
    end

    self._session_data = session_data
    self._primary_context_cache = nil
    return true
end

function session_reader:state()
    return {
        session_id = self._session_data.session_id,
        user_id = self._session_data.user_id,
        status = self._session_data.status,
        title = self._session_data.title,
        kind = self._session_data.kind,
        meta = self._session_data.meta or {},
        config = self._session_data.config or {},
        public_meta = self._session_data.public_meta or {},
        start_date = self._session_data.start_date,
        last_message_date = self._session_data.last_message_date,
        primary_context_id = self._session_data.primary_context_id
    }
end

function session_reader:primary_context_id()
    return self._session_data.primary_context_id
end

-- PRIMARY CONTEXT OPERATIONS (JSON key-value storage)

function session_reader:_load_primary_context()
    if self._primary_context_cache then
        return self._primary_context_cache
    end

    local context, err = session._context_repo.get(self._session_data.primary_context_id)
    if err then
        return nil, "Failed to get primary context: " .. err
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

    self._primary_context_cache = data
    return data
end

function session_reader:get_context(key)
    if not key then
        return nil, "Context key is required"
    end

    local data, err = self:_load_primary_context()
    if err then
        return nil, err
    end

    return data[key]
end

function session_reader:get_full_context()
    return self:_load_primary_context()
end

function session_reader:messages()
    local query = setmetatable({}, message_query)
    query._session_id = self.session_id
    query._reader = self
    query._type_filter = nil
    query._limit = nil
    query._offset = nil
    query._after_message_id = nil
    query._error = nil
    return query
end

function session_reader:artifacts()
    local query = setmetatable({}, artifact_query)
    query._session_id = self.session_id
    query._kind_filter = nil
    query._limit = nil
    query._offset = nil
    query._error = nil
    return query
end

function session_reader:contexts()
    local query = setmetatable({}, context_query)
    query._session_id = self.session_id
    query._type_filter = nil
    query._error = nil
    return query
end

-- MESSAGE QUERY BUILDER

function message_query:type(msg_type)
    if not msg_type then
        self._error = "Message type is required"
        return self
    end
    self._type_filter = msg_type
    return self
end

function message_query:last(limit)
    if not limit or limit <= 0 then
        self._error = "Limit must be positive number"
        return self
    end
    self._limit = limit
    return self
end

function message_query:offset(offset)
    if not offset or offset < 0 then
        self._error = "Offset must be non-negative number"
        return self
    end
    self._offset = offset
    return self
end

function message_query:after(message_id)
    if not message_id then
        self._error = "Message ID is required"
        return self
    end
    self._after_message_id = message_id
    return self
end

function message_query:from_checkpoint()
    if not self._reader then
        self._error = "Reader reference missing for checkpoint query"
        return self
    end

    -- Load the current checkpoint ID from session context
    local checkpoint_id = self._reader:get_context(consts.CONTEXT_KEYS.CURRENT_CHECKPOINT_ID)

    if checkpoint_id then
        self._after_message_id = checkpoint_id
    end

    return self
end

function message_query:all()
    if self._error then
        return nil, self._error
    end

    local messages, err

    if self._after_message_id then
        messages, err = session._message_repo.list_after_message(self._session_id, self._after_message_id, self._limit)
    elseif self._type_filter then
        messages, err = session._message_repo.list_by_type(self._session_id, self._type_filter, self._limit, self
        ._offset)
    else
        local result
        result, err = session._message_repo.list_by_session(self._session_id, self._limit)
        if result then
            messages = result.messages
        end
    end

    if err then
        return nil, "Failed to fetch messages: " .. err
    end

    return messages or {}, nil
end

function message_query:one()
    if self._error then
        return nil, self._error
    end

    local original_limit = self._limit
    self._limit = 1
    local results, err = self:all()
    self._limit = original_limit

    if err then
        return nil, err
    end

    return results and results[1] or nil, nil
end

function message_query:count()
    if self._error then
        return nil, self._error
    end

    if self._type_filter then
        local count, err = session._message_repo.count_by_type(self._session_id, self._type_filter)
        if err then
            return nil, "Failed to count messages: " .. err
        end
        return count, nil
    else
        local count, err = session._message_repo.count_by_session(self._session_id)
        if err then
            return nil, "Failed to count messages: " .. err
        end
        return count, nil
    end
end

-- ARTIFACT QUERY BUILDER

function artifact_query:kind(artifact_kind)
    if not artifact_kind then
        self._error = "Artifact kind is required"
        return self
    end
    self._kind_filter = artifact_kind
    return self
end

function artifact_query:last(limit)
    if not limit or limit <= 0 then
        self._error = "Limit must be positive number"
        return self
    end
    self._limit = limit
    return self
end

function artifact_query:offset(offset)
    if not offset or offset < 0 then
        self._error = "Offset must be non-negative number"
        return self
    end
    self._offset = offset
    return self
end

function artifact_query:all()
    if self._error then
        return nil, self._error
    end

    local artifacts, err

    if self._kind_filter then
        artifacts, err = session._artifact_repo.list_by_kind(self._session_id, self._kind_filter, self._limit,
            self._offset)
    else
        artifacts, err = session._artifact_repo.list_by_session(self._session_id, self._limit, self._offset)
    end

    if err then
        return nil, "Failed to fetch artifacts: " .. err
    end

    return artifacts or {}, nil
end

function artifact_query:one()
    if self._error then
        return nil, self._error
    end

    local original_limit = self._limit
    self._limit = 1
    local results, err = self:all()
    self._limit = original_limit

    if err then
        return nil, err
    end

    return results and results[1] or nil, nil
end

function artifact_query:count()
    if self._error then
        return nil, self._error
    end

    if self._kind_filter then
        local count, err = session._artifact_repo.count_by_kind(self._session_id, self._kind_filter)
        if err then
            return nil, "Failed to count artifacts: " .. err
        end
        return count, nil
    else
        local count, err = session._artifact_repo.count_by_session(self._session_id)
        if err then
            return nil, "Failed to count artifacts: " .. err
        end
        return count, nil
    end
end

-- CONTEXT QUERY BUILDER (for session_contexts table - type + text)

function context_query:type(context_type)
    if not context_type then
        self._error = "Context type is required"
        return self
    end
    self._type_filter = context_type
    return self
end

function context_query:all()
    if self._error then
        return nil, self._error
    end

    local contexts, err

    if self._type_filter then
        contexts, err = session._session_contexts_repo.list_by_type(self._session_id, self._type_filter)
    else
        contexts, err = session._session_contexts_repo.list_by_session(self._session_id)
    end

    if err then
        return nil, "Failed to fetch contexts: " .. err
    end

    return contexts or {}, nil
end

function context_query:one()
    if self._error then
        return nil, self._error
    end

    local results, err = self:all()
    if err then
        return nil, err
    end
    return results and results[1] or nil, nil
end

function context_query:count()
    if self._error then
        return nil, self._error
    end

    if self._type_filter then
        local count, err = session._session_contexts_repo.count_by_type(self._session_id, self._type_filter)
        if err then
            return nil, "Failed to count contexts: " .. err
        end
        return count, nil
    else
        local count, err = session._session_contexts_repo.count_by_session(self._session_id)
        if err then
            return nil, "Failed to count contexts: " .. err
        end
        return count, nil
    end
end

return session
