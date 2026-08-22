-- time.parse answers two normal arms: an instant with no error, or nil with the
-- module's error. Reading a method off the first slot without ruling the nil
-- arm out uses a value the declaration says may be nil.
local time = require("time")

local function stamp(text: string): string
    local parsed = time.parse(time.RFC3339, text)
    return parsed:format_rfc3339()
end

return stamp
