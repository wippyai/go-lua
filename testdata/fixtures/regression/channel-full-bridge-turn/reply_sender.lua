-- kickside.connection:reply_sender stand-in: records each send_reply call and
-- returns a fixed result, so channel_sink_test exercises the sink's dispatch
-- without booting the connection module's provider resolution.
local M = {}

local calls = {}

function M.reset()
    calls = {}
end

function M.calls()
    return calls
end

function M.send_reply(worker_config, content)
    calls[#calls + 1] = { worker_config = worker_config, content = content }
    return { sent = true, message_ref = "stub-message" }, nil
end

function M.update_status(handle, content)
    calls[#calls + 1] = { worker_config = handle, content = content, update_status = true }
    handle.sent = true
    handle.editable = true
    handle.message_ref = handle.message_ref or "stub-message"
    return handle, nil, true
end

function M.finish_status(handle, content)
    calls[#calls + 1] = { worker_config = handle, content = content, finish_status = true }
    return { sent = true, message_ref = handle.message_ref or "stub-message" }, nil, true
end

function M.send_status(worker_config, phase, label)
    calls[#calls + 1] = { worker_config = worker_config, phase = phase, label = label, send_status = true }
    return true, nil
end

return M

