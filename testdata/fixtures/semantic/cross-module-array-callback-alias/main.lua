local builder = require("builder")
local protocol = require("protocol")

local function map<T, U>(items: {T}, fn: (T) -> U): {U}
    local out: {U} = {}
    for _, item in ipairs(items) do
        table.insert(out, fn(item))
    end
    return out
end

local metrics = {
    builder.metric("latency", 42, {source = "api"}),
    builder.metric("errors", 0, {source = "worker"}),
}

local events: {protocol.Event} = map(metrics, function(metric: protocol.Metric)
    return builder.event(metric)
end)

local first = events[1]
if first then
    local kind: "metric" | "log" = first.kind
    local name: string = first.name
    local bad_name: number = first.name -- expect-error
    print(kind .. ":" .. name)
end

local wrong_events: {protocol.Metric} = map(metrics, function(metric: protocol.Metric) -- expect-error
    return builder.event(metric)
end)
