local protocol = require("protocol")

type DecodeResult = {ok: true, value: protocol.Request} | {ok: false, error: string}

local M = {}

function M.decode(raw: any): DecodeResult
    if type(raw) ~= "table" then
        return {ok = false, error = "root"}
    end
    if type(raw.id) ~= "string" then
        return {ok = false, error = "id"}
    end
    if type(raw.retries) ~= "number" then
        return {ok = false, error = "retries"}
    end
    local tags: {[string]: string} = {}
    if type(raw.tags) == "table" then
        for key, value in pairs(raw.tags) do
            if type(key) == "string" and type(value) == "string" then
                tags[key] = value
            end
        end
    end
    return {ok = true, value = {id = raw.id, retries = raw.retries, tags = tags}}
end

return M
