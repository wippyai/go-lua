local api = require("api")
local types = require("types")

local M = {}

function M.connect(component_id: string?): (types.Conn?, string?)
    return nil, "no Fireflies connection selected"
end

M.update_meeting_title = api.update_meeting_title

return M
