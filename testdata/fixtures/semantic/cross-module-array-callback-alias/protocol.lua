type Event = {
    kind: "metric" | "log",
    name: string,
    tags: {[string]: string},
}

type Metric = {
    name: string,
    value: number,
    tags: {[string]: string},
}

local M = {}
M.Event = Event
M.Metric = Metric

return M
