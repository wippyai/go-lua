local protocol = require("protocol")
local ready: boolean = protocol.kind -- expect-error: "ready"

return ready
