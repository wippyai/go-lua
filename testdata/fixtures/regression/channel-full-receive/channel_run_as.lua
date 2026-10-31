-- run_as stub for the receive witness: resolves a frozen installer spec to a fake
-- (actor, scope) pair the thread-bridge stub carries opaquely, so the ingress
-- witness path is exercised without the execution-identity runtime. When
-- test_state.state.witness_run_as_err is set (an installer principal that no
-- longer resolves), it returns that error so the witness path's truthful skip is
-- observable to a test.
local test_state = require("test_state")

local M = {}

function M.resolve(spec)
    if type(spec) ~= "table" then return nil, nil, "run_as spec is required" end
    if test_state.state.witness_run_as_err then
        return nil, nil, test_state.state.witness_run_as_err
    end
    local identity = type(spec.identity) == "table" and spec.identity or {}
    local actor_id = type(identity.actor_id) == "string" and identity.actor_id or "installer-1"
    local actor = { id = function(_self) return actor_id end }
    local scope = { id = actor_id }
    return actor, scope, nil
end

return M

