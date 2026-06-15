local protocol = require("protocol")

local M = {}

function M.new_source(
    messages: Channel<protocol.Message>,
    ticks: Channel<protocol.Tick>
): protocol.SourceBox
    return protocol.box_source({
        messages = messages,
        ticks = ticks,
    })
end

return M
