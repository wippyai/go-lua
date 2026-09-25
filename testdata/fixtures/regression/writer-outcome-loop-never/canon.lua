-- Canonical JSON + command payload hashing. Object keys sort recursively, arrays
-- keep index order, whitespace is elided, so a payload_hash is stable across hosts
-- and dialects. Two submissions of the same command_id are a replay iff their
-- canonical payload hashes match; a mismatch is a conflict.

local json = require("json")
local hash = require("hash")

local M = {}

function M.encode(value: any): string
    local t = type(value)
    if t == "nil" or t == "string" or t == "number" or t == "boolean" then
        return json.encode(value)
    end
    if t ~= "table" then return json.encode(tostring(value)) end
    local is_array = true
    local max = 0
    for k, _ in pairs(value :: any) do
        if type(k) ~= "number" or k < 1 or math.floor(k) ~= k then is_array = false; break end
        if k > max then max = k end
    end
    if is_array then
        local parts: { string } = {}
        for i = 1, max do parts[#parts + 1] = M.encode((value :: any)[i]) end
        return "[" .. table.concat(parts, ",") .. "]"
    end
    local keys: { string } = {}
    for k, _ in pairs(value :: any) do keys[#keys + 1] = tostring(k) end
    table.sort(keys)
    local parts: { string } = {}
    for _, k in ipairs(keys) do
        parts[#parts + 1] = json.encode(k) .. ":" .. M.encode((value :: any)[k])
    end
    return "{" .. table.concat(parts, ",") .. "}"
end

function M.hash(value: any): string
    return tostring(hash.sha256(M.encode(value)))
end

return M

