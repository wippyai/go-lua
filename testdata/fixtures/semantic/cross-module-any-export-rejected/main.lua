local protocol = require("protocol")
local provider = require("provider")

local raw = provider.raw_config()
local config: protocol.Config = raw -- expect-error

if raw.id then
    local id: string = raw.id -- expect-error
end

return "ok"
