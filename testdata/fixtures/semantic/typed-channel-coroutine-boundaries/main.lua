local protocol = require("protocol")
local runtime = require("runtime")

type Decoder<T> = (any) -> T
type ListenOptions<T> = {
    channel: Channel<T>,
    decode: Decoder<T>,
}

local function decode_event(raw: any): protocol.Event
    return {
        kind = "event",
        id = tostring(raw),
        attempt = 1,
    }
end

local function decode_timer(raw: any): protocol.Timer
    return {
        kind = "timer",
        elapsed = 1,
    }
end

local function listen<T>(topic: string, options: ListenOptions<T>): Channel<T>
    return options.channel
end

local function consume_source(source: protocol.EventSource): string
    local direct: string = runtime.read_event(source.primary)
    local selected: string = runtime.select_source(source.primary, source.retry, source.timers, source.stops)

    runtime.spawn_event_worker(source)
    runtime.spawn_timer_worker(source)

    local result = channel.select {
        source.primary:case_receive(),
        source.timers:case_receive(),
        source.stops:case_receive(),
    }

    if result.channel == source.primary then
        local event = result.value
        local id: string = event.id
        local wrong: number = event.id -- expect-error
        direct = direct .. id
    end

    if result.channel == source.timers then
        local timer = result.value
        local elapsed: number = timer.elapsed
        local wrong: string = timer.elapsed -- expect-error
        selected = selected .. tostring(elapsed)
    end

    local event, event_ok = source.retry:receive()
    if event_ok then
        local label: string = protocol.event_label(event)
        local wrong: number = protocol.event_label(event) -- expect-error
        direct = direct .. label
    end

    local timer, timer_ok = source.timers:receive()
    if timer_ok then
        local label: string = protocol.timer_label(timer)
        selected = selected .. label
    end

    local handler: (protocol.Event) -> string = function(event: protocol.Event): string
        return protocol.event_label(event)
    end

    local wrong_handler: (protocol.Event) -> string = function(timer: protocol.Timer): string -- expect-error
        return protocol.timer_label(timer)
    end

    local typed_events: Channel<protocol.Event> = listen("events", {
        channel = source.primary,
        decode = decode_event,
    })
    local typed_event, typed_event_ok = typed_events:receive()
    if typed_event_ok then
        local id: string = typed_event.id
        local wrong: number = typed_event.id -- expect-error
        direct = direct .. id
    end

    local inferred_events = listen("events", {
        channel = source.primary,
        decode = decode_event,
    })
    local inferred_event, inferred_event_ok = inferred_events:receive()
    if inferred_event_ok then
        local id: string = inferred_event.id
        local wrong: number = inferred_event.id -- expect-error
        direct = direct .. id
    end

    local inferred_timers = listen("timers", {
        channel = source.timers,
        decode = decode_timer,
    })
    local inferred_timer, inferred_timer_ok = inferred_timers:receive()
    if inferred_timer_ok then
        local elapsed: number = inferred_timer.elapsed
        local wrong: string = inferred_timer.elapsed -- expect-error
        selected = selected .. tostring(elapsed)
    end

    local wrong_typed_events: Channel<protocol.Event> = listen("events", { -- expect-error
        channel = source.primary,
        decode = decode_timer,
    })

    return direct .. selected .. handler(event)
end

return consume_source
