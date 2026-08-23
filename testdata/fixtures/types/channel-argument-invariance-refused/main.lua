type Event = { id: string }
type Timer = { at: number }

local function sink(out: Channel<Timer>)
    return out
end

local function feed(inbox: Channel<Event>)
    return sink(inbox)
end

return feed
