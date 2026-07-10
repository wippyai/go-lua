local funcs = require("funcs")
local security = require("security")
local target = require("target")

local function dispatch(input: target.Input): (target.RunResult?, string?)
    return funcs.new()
        :with_actor(security.new_actor("repro_user", {security_groups = {"security:process"}}))
        :with_scope(security.named_scope("security:process"))
        :with_context({})
        :call("ns:target", input)
end

local result, err = dispatch({
    title = "first",
    context = {trace_id = "trace-1"},
})

if err then
    error(err)
end
if not result then
    error("missing result")
end
local actor_scope_value = result.metadata.actor_scope
if not actor_scope_value then
    error("missing actor scope")
end
local trace_id_value = result.state.trace_id
if not trace_id_value then
    error("missing trace id")
end

local schedule_id: string = result.state.schedule_id
local actor_scope: string = actor_scope_value
local trace_id: string = trace_id_value
