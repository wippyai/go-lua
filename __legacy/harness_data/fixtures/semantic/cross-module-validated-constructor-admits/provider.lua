local protocol = require("protocol")

local M = {}

function M.decode(raw: any): protocol.DecodeResult
    if type(raw) ~= "table" then
        return {ok = false, error = "root"}
    end
    if type(raw.id) ~= "string" then
        return {ok = false, error = "id"}
    end
    if type(raw.retries) ~= "number" then
        return {ok = false, error = "retries"}
    end
    return {ok = true, value = {id = raw.id, retries = raw.retries}}
end

return M
