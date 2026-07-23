local protocol = require("protocol")
local runtime = require("runtime")
local validator = require("validator")

local finish = runtime.route("finish", "finish", nil)
local middle = runtime.route("middle", "middle", finish)
local start = runtime.route("start", "start", middle)

local actor = runtime.new_actor("supervisor")
actor:add_route(start)
actor:add_route(middle)
actor:add_route(finish)
actor:register("task", runtime.task_handler)
actor:register("timer", runtime.timer_handler)

local raw_messages: {any} = {
    {kind = "task", id = "m1", route_id = "start", payload = {owner = "ops", retries = 3}},
    {kind = "timer", id = "m2", due_at = 10},
    {kind = "task", id = 404, route_id = "start"},
}

local outputs: {string} = {}
for _, raw in ipairs(raw_messages) do
    local decoded = validator.decode(raw)
    if decoded.ok then
        local result = actor:dispatch(decoded.value)
        if result.ok then
            table.insert(outputs, result.value)
        else
            table.insert(outputs, result.error.code)
        end
    else
        table.insert(outputs, decoded.error.code)
    end
end

local last_id = actor.state.last_id
if last_id then
    local last: string = last_id
end

local processed = actor.state.processed["m1"]
if processed and processed.kind == "task" then
    local route_id: string = processed.route_id
    local owner = processed.payload.owner
    if owner then
        local owner_name: string = owner
    end
end

local unknown_raw: any = {kind = "task", id = 5, route_id = "start"}
local trusted: protocol.TaskMessage = unknown_raw -- expect-error
if unknown_raw.kind == "task" then
    local route_id: string = unknown_raw.route_id -- expect-error
end

local missing_processed: protocol.Envelope = actor.state.processed["missing"] -- expect-error
local missing_counter: number = actor.state.counters["missing"] -- expect-error

print(outputs[1])
print(outputs[2])
print(outputs[3])
