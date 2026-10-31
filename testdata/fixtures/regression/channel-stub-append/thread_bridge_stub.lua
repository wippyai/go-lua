local test_state = require("test_state")

-- thread_bridge stub: captures the ingress witness (ensure_thread + append_inbound)
-- so a test asserts WHICH inbound was recorded, under WHICH identity, without the
-- real kickside.core threads runtime. Mirrors the real signatures, including the
-- explicit (actor, scope) the ingress witness passes.
local M = {}

function M.ensure_thread(inbound, opts, actor, scope)
    test_state.state.witness[#test_state.state.witness + 1] = {
        kind = "ensure_thread",
        inbound = inbound,
        opts = opts,
        actor_id = type(actor) == "table" and actor:id() or nil,
        has_scope = scope ~= nil,
    }
    if test_state.state.witness_ensure_err then
        return nil, nil, test_state.state.witness_ensure_err
    end
    return "thread-witness-1", { thread_id = "thread-witness-1" }, nil
end

function M.append_inbound(thread_id, inbound, opts, actor, scope)
    test_state.state.witness[#test_state.state.witness + 1] = {
        kind = "append_inbound",
        thread_id = thread_id,
        inbound = inbound,
        opts = opts,
        actor_id = type(actor) == "table" and actor:id() or nil,
        has_scope = scope ~= nil,
    }
    local state = test_state.state
    local msg_id = type(inbound) == "table" and tostring(inbound.external_message_id or "") or ""
    if state.durable_append_dedupe == true and msg_id ~= "" then
        state.durable_seen = type(state.durable_seen) == "table" and state.durable_seen or {}
        if state.durable_seen[msg_id] == true then
            return { duplicate = true, inserted = 0 }, nil
        end
        if type(state.append_errors) == "table" and state.append_errors[msg_id] ~= nil then
            return nil, state.append_errors[msg_id]
        end
        state.durable_seen[msg_id] = true
    end
    if type(state.append_duplicate_ids) == "table" and msg_id ~= "" and state.append_duplicate_ids[msg_id] == true then
        return { duplicate = true, inserted = 0 }, nil
    end
    if type(state.append_errors) == "table" and msg_id ~= "" and state.append_errors[msg_id] ~= nil then
        return nil, state.append_errors[msg_id]
    end
    return { inserted = 1, id = "event-" .. tostring(#test_state.state.witness) }, nil
end

return M

