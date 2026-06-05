local protocol = require("protocol")

local M = {}

local function map_receive<T>(ch: Channel<T>, fn: (T) -> string): string
    local value, ok = ch:receive()
    if ok then
        return fn(value)
    end
    return "closed"
end

function M.read_event(ch: Channel<protocol.Event>): string
    return map_receive(ch, function(event: protocol.Event): string
        local id: string = event.id
        return id
    end)
end

function M.select_source(
    primary: Channel<protocol.Event>,
    retry: Channel<protocol.Event>,
    timers: Channel<protocol.Timer>,
    stops: Channel<protocol.Stop>
): string
    local result = channel.select {
        primary:case_receive(),
        retry:case_receive(),
        timers:case_receive(),
        stops:case_receive(),
    }

    if result.channel == primary then
        local event = result.value
        local id: string = event.id
        return "primary:" .. id
    end

    if result.channel == retry then
        local event = result.value
        local attempt: number = event.attempt
        return "retry:" .. tostring(attempt)
    end

    if result.channel == timers then
        local timer = result.value
        local elapsed: number = timer.elapsed
        return "timer:" .. tostring(elapsed)
    end

    local stop = result.value
    local reason: string = stop.reason
    return "stop:" .. reason
end

function M.spawn_event_worker(source: protocol.EventSource)
    coroutine.spawn(function()
        local event, ok = source.primary:receive()
        if ok then
            local id: string = event.id
            protocol.event_label(event)
        end
    end)
end

function M.spawn_timer_worker(source: protocol.EventSource)
    local function worker()
        local timer, ok = source.timers:receive()
        if ok then
            local elapsed: number = timer.elapsed
            protocol.timer_label(timer)
        end
    end
    coroutine.spawn(worker)
end

return M
