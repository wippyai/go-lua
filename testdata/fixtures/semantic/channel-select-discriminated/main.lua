type A = { tag: "a", n: number }
type B = { tag: "b", s: string }
local function pick(ca: Channel<A>, cb: Channel<B>): string
    local sel = channel.select { ca:case_receive(), cb:case_receive() }
    if sel.channel == ca then
        local v = sel.value
        return tostring(v.n)
    end
    return "b"
end
