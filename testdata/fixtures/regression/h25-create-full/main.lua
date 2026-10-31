local create_service = require("create_service")
local api_access = require("api_access")

local M = {}

type Args = { bootstrap_crm_id: unknown? }

function M.run(args: unknown): (unknown?, string?)
    if type(args) == "table" and type((args :: Args).bootstrap_crm_id) == "string" then
        return api_access.bootstrap_payload((args :: Args).bootstrap_crm_id)
    end
    return create_service.create(args)
end

return M
