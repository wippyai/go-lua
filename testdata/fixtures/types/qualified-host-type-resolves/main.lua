local stream = require("stream")

local function identifier(handle: stream.Stream): string
    return handle.id
end

return identifier
