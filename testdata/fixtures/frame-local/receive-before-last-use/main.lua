type Message = {
    value: integer,
}

local function route(ch: Channel<Message>): integer
    local scratch = {
        value = 1,
    }
    local _, ok = ch:receive()
    if ok then
        return scratch.value
    end
    return scratch.value
end

local out: integer = route(nil :: Channel<Message>)
