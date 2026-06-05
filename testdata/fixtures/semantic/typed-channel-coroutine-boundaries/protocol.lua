type Event = {
    kind: "event",
    id: string,
    attempt: number,
}

type Timer = {
    kind: "timer",
    elapsed: number,
}

type Stop = {
    kind: "stop",
    reason: string,
}

type EventSource = {
    primary: Channel<Event>,
    retry: Channel<Event>,
    timers: Channel<Timer>,
    stops: Channel<Stop>,
}

local M = {}

function M.event_label(event: Event): string
    return event.id .. ":" .. tostring(event.attempt)
end

function M.timer_label(timer: Timer): string
    return tostring(timer.elapsed)
end

return M
