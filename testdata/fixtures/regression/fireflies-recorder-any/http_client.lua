type Response = { status_code: integer, body: string? }

local M = {}

function M.request(method: string, url: string, options: any?): (Response?, string?)
    return nil, "offline"
end

return M
