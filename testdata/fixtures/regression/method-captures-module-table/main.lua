-- A method defined with `:` sees the module-level tables it captures, as a
-- plain function does (session reader: context_query:all() reads the contexts
-- repository through the module's session table).
local repo = {}
function repo.list_by_type(session_id: string?, context_type: string?)
    if not session_id then
        return nil, "Session ID is required"
    end
    return { { text = "summary" } }, nil
end

local session = {
    _repo = repo,
}

local context_query = {
    _session_id = nil :: string?,
    _type_filter = nil :: string?,
}
context_query.__index = context_query

function context_query:type(context_type: string)
    self._type_filter = context_type
    return self
end

function context_query:first_text(): string
    local contexts, err = session._repo.list_by_type(self._session_id, self._type_filter)
    if err or not contexts then
        return ""
    end
    local first = contexts[1]
    return first.text
end

return context_query
