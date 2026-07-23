type Conn = { socket: { addr: string }? }
local function host(c: Conn?): string
    if c ~= nil and c.socket ~= nil then
        return c.socket.addr
    end
    return "none"
end
return host({ socket = { addr = "1.2.3.4" } })
