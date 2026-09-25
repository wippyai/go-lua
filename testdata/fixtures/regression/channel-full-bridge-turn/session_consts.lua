local M = {}

M.PROCESS = { SESSION_ID = "wippy.session.process:session" }
M.TOPICS = { MESSAGE = "session.message" }
M.TOPIC_PREFIXES = { SESSION = "session:" }
M.UPSTREAM_TYPES = {
    CONTENT = "content",
    UPDATE = "update",
    ERROR = "error",
    FUNCTION_CALL = "function_call",
    FUNCTION_ERROR = "function_error",
}
M.STATUS = { IDLE = "idle" }

function M.get_config()
    return {}
end

return M

