local events = require("events")
local sessions = require("sessions")
local request_builder = require("request_builder")
local pipeline = require("pipeline")

local rendered, event_errors = events.collect({
    {kind = "message", id = "m1", text = "hello", tags = {"ui", "chat"}},
    {kind = "tool", id = "t1", name = "search", arguments = {query = "lua"}},
    {kind = "error", id = "e1", code = "E", message = "boom"},
})

local session_text, session_err = sessions.describe("u1", 10)
if session_err then
    return session_err.message
end

local request = request_builder.new()
    :with_method("POST")
    :with_header("Accept", "application/json")
    :with_query("q", rendered[1])
    :with_timeout(nil)
    :build()

local flow = pipeline.enable_defaults(pipeline.new("deploy"), {
    REGION = "local",
    LOG_LEVEL = "debug",
})

local first = flow.plugins[1]
if first then
    local plugin_id: string = first.id
    local level = first.config.level
    if type(level) == "string" then
        local plugin_level: string = level
    end
end

local summary: string = rendered[1] .. ":" .. session_text .. ":" .. request.headers.Accept .. ":" .. flow.env.REGION -- expect-warning: may be nil

local options: {timeout: string?} = {timeout = "30s"}
local timeout = options.timeout or 30
local bad_timeout: number = timeout -- expect-error

local meta: string | {content_type: string} = ""
local artifact = {meta = meta}
if artifact.meta then
    local content_type: string = artifact.meta.content_type -- expect-error
end

return summary .. ":" .. tostring(event_errors[1])
