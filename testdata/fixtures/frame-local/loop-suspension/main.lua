type Message = {
    value: integer,
}

local function route(ch: Channel<Message>): integer
    local total = 0
    for i = 1, 2 do
        local scratch = {
            value = i,
        }
        local _, _ = ch:receive()
        total = total + scratch.value
    end
    return total
end

local out: integer = route(nil :: Channel<Message>)
