type A = { a: string }
type B = { b: string }

local function wait(x: channel.Channel<A>, y: channel.Channel<B>, flag: boolean)
    local ch: channel.Channel<A> | channel.Channel<B> = x
    if not flag then
        ch = y
    end
    local timeout = time.after("1s")
    local r = channel.select({ ch:case_receive(), timeout:case_receive() })
    if r.channel == timeout then
        return nil
    end
    local v: A | B = r.value
    local t: time.Time = r.value -- expect-error: cannot assign
    return v
end

return wait
