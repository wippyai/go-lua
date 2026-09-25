local test_state = require("test_state")

local M = {}

-- runtime_reader stub: get_runtime returns one installed responder's private state
-- and frozen execution identity by component id (the single-component read the
-- indexed routing path uses after the meta lookup resolves the id).
local function runtime_reader()
    return {
        open = function()
            return {
                get = function(_self, args)
                    local rid = type(args) == "table" and tostring(args.id or "") or ""
                    local record = test_state.state.responders[rid]
                    if not record then return { success = false, error = "automation not found" }, nil end
                    return {
                        success = true,
                        automation = {
                            id = rid,
                            state = record.state,
                            execution_identity = record.execution_identity,
                        },
                    }, nil
                end,
            }, nil
        end,
    }
end

-- threads stub: find returns the single seeded thread row (attrs already a table,
-- as the single-thread read hydrates it).
local function threads()
    return {
        open = function()
            return {
                find = function(_self, _args)
                    return test_state.state.thread, test_state.state.thread_find_err
                end,
            }, nil
        end,
    }
end

-- receive-target stub: captures the inbound inject re-dispatches and returns the
-- registered result, so a test can assert WHICH declared contract inject resolved.
local function receive_target(id, result)
    return {
        open = function()
            return {
                receive = function(_self, inbound)
                    table.insert(test_state.state.receive_calls, { target = id, inbound = inbound })
                    return result
                end,
            }, nil
        end,
    }
end

function M.get(id)
    if id == "kickside.contract:runtime_reader" then return runtime_reader(), nil end
    if id == "kickside.core:threads" then return threads(), nil end
    local result = test_state.state.receive_targets[id]
    if result ~= nil then return receive_target(id, result), nil end
    return nil, "unknown contract: " .. tostring(id)
end

return M

