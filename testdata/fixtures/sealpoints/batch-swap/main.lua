type Event = {
    kind: string,
    seq: number,
}

local function newFlusher(out: Channel<{Event}>)
    local pending: {Event} = {}

    local function record(e: Event)
        pending[#pending + 1] = e
    end

    local function flush()
        local batch = pending
        pending = {}
        out:send(batch)
    end

    return record, flush
end
