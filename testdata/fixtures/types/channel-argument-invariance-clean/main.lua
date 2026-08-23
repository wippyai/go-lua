type Event = { id: string }

local function sink(out: Channel<Event>)
    return out
end

local function feed(inbox: Channel<Event>)
    return sink(inbox)
end

return feed
