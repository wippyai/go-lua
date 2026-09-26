local ch = channel.new(1)
local timeout = time.after("1s")
local r = channel.select({ ch:case_receive(), timeout:case_receive() })
if r.channel == timeout then
    local fired: time.Time = r.value
    return fired:unix()
else
    local msg = r.value
    local t: time.Time = r.value -- expect-error: cannot assign unknown
    return msg.field
end
