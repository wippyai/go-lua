local ch = channel.new(1)
local timeout = time.after("1s")
local r = channel.select({ ch:case_receive(), timeout:case_receive() })
local v = r.value
local t: time.Time = r.value -- expect-error: cannot assign unknown
return v.field
