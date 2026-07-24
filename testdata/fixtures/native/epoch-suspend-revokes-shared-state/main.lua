-- A table_element fact read from an actor-visible module table does not survive a channel
-- receive: another actor may run across the suspension point.

local shared: {number} = { 1, 2, 3 }

local function drain(ch: Channel<string>): number
    local before = shared[1]
    local _, _ = ch:receive()
    local after = shared[1]
    return (before or 0) + (after or 0)
end

return drain
