local a = channel.new(1)
local b = channel.new(1)
local timeout = time.after("1s")
local r = channel.select({ a:case_receive(), b:case_receive(), timeout:case_receive() })
if r.channel == timeout then
    return nil
end
local msg = r.value
local t: time.Time = r.value -- expect-error: cannot assign unknown
return msg.field
