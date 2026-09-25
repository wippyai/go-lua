local M = {}

M.HUB_NAME = "kickside.channel.session_hub"
M.HUB_HOST = "app:processes"
M.HUB_TOPIC = "kickside.channel.inbound"
M.SESSION_MAP_TABLE = "kickside_channel_sessions"
M.DEFAULT_RUNTIME_IDLE_SECONDS = 10 * 60
M.DEFAULT_SESSION_IDLE_SECONDS = 2 * 60 * 60
M.THREAD_CLASS = "kickside.channel"
M.RESPONDER_THREAD_CLASS = "automation"

M.RUN_AS = {
    SUBJECT = "subject",
    FROZEN = "frozen",
}

M.EVENT = {
    INBOUND = "kickside.channel.events:message.inbound",
    REPLY_SENT = "kickside.channel.events:reply.sent",
    REPLY_FAILED = "kickside.channel.events:reply.failed",
}

function M.db_id()
    return "app:db", nil
end

function M.external_ref(provider, component_id, channel_id)
    return provider .. ":" .. component_id .. ":" .. channel_id
end

function M.event_external_source(provider)
    return "kickside.channel." .. provider
end

return M

