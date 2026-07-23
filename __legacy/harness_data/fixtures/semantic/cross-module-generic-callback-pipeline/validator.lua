local protocol = require("protocol")
local result = require("result")

type UserResult = {ok: true, value: protocol.User} | {ok: false, error: string}

local M = {}

function M.decode_user(raw: any): UserResult
    if type(raw) ~= "table" then
        return result.err("root")
    end
    if type(raw.id) ~= "string" then
        return result.err("id")
    end
    if type(raw.retries) ~= "number" then
        return result.err("retries")
    end
    return result.ok({id = raw.id, retries = raw.retries})
end

return M
