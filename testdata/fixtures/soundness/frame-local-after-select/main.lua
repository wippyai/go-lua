-- A table allocated before channel.select remains reachable after the
-- suspension point, so it must not be classified FrameLocal.
type Message = { value: integer }

local function route(ch: Channel<Message>): integer
    local scratch = { value = 1 }
    local selected = channel.select {
        ch:case_receive(),
    }
    local out: integer = scratch.value
    if selected.ok then
        return out
    end
    return out
end

local out: integer = route(nil :: Channel<Message>)
return out
