local test_state = require("test_state")

local M = {}

function M.append_inbound(thread_id, inbound)
    test_state.state.witness[#test_state.state.witness + 1] = {
        kind = "append_inbound",
        thread_id = thread_id,
        inbound = inbound,
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

test_state.reset({ witness = {}, durable_append_dedupe = true, append_errors = { m1 = "boom" } })
local res, err = M.append_inbound("t1", { external_message_id = "m1" })
assert(res == nil and err == "boom")
local res2 = M.append_inbound("t1", { external_message_id = "m2" })
assert(res2 ~= nil and res2.inserted == 1)
