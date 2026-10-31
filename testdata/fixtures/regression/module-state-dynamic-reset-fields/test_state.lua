local M = {}

local function fresh()
    return {
        links = {},
        user_links = {},
        link_agents = {},
        replies = {},
        sent = {},
        link_calls = {},
        clears = {},
        sessions = {},
        send_error = nil,
        link_code_error = nil,
        link_code_result = nil,
        next_session_id = "session-1",
    }
end

M.state = fresh()

function M.reset(values)
    M.state = fresh()
    if type(values) == "table" then
        for k, v in pairs(values) do
            M.state[k] = v
        end
    end
    return M.state
end

function M.session_key(provider, external_id)
    return tostring(provider or "") .. "\n" .. tostring(external_id or "")
end

function M.link_key(provider, external_id)
    return tostring(provider or "") .. "\n" .. tostring(external_id or "")
end

return M
