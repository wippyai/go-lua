local protocol = require("protocol")

local M = {}

function M.metric(name: string, value: number, tags: {[string]: string}): protocol.Metric
    local metric: protocol.Metric = {name = name, value = value, tags = tags}
    return metric
end

function M.event(metric: protocol.Metric): protocol.Event
    local event: protocol.Event = {
        kind = "metric",
        name = metric.name,
        tags = metric.tags,
    }
    return event
end

return M
