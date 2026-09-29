local json = require("json")

type JsonValue = nil | boolean | number | string | { JsonValue } | { [string]: JsonValue }
type JsonObject = { [string]: JsonValue }
type Seen = { [unknown]: boolean }

local M = {}

local REDACTED = "[redacted]"

local function normalized_key(key: unknown): string
    return tostring(key or ""):lower():gsub("[^a-z0-9]", "")
end

local function is_sensitive_key(key: unknown): boolean
    local name = normalized_key(key)
    if name == "" then return false end
    return name:find("token", 1, true) ~= nil
        or name:find("secret", 1, true) ~= nil
        or name:find("password", 1, true) ~= nil
        or name:find("apikey", 1, true) ~= nil
        or name:find("authorization", 1, true) ~= nil
        or name:find("credential", 1, true) ~= nil
        or name:find("cookie", 1, true) ~= nil
end

local function utf8_char_len(first_byte: integer): integer
    if first_byte < 128 then return 1 end
    if first_byte < 224 then return 2 end
    if first_byte < 240 then return 3 end
    if first_byte < 248 then return 4 end
    return 1
end

local function safe_prefix(text: string, max_bytes: number): string
    local limit = math.floor(max_bytes)
    if #text <= limit then return text end
    local n = limit
    while n > 0 do
        local b = string.byte(text, n)
        if not b or b < 128 or b >= 192 then break end
        n = n - 1
    end
    local b = n > 0 and string.byte(text, n) or nil
    if b and b >= 128 and n + utf8_char_len(b) - 1 > limit then
        n = n - 1
    end
    if n < 1 then return "" end
    return text:sub(1, n)
end

local function sanitize(value: JsonValue, seen: Seen?): JsonValue
    if type(value) ~= "table" then return value end
    seen = seen or {}
    if seen[value] then return "[circular]" end
    seen[value] = true

    local source = value :: JsonObject
    local out: JsonObject = {}
    for key, item in pairs(source) do
        if is_sensitive_key(key) then
            out[key] = REDACTED
        else
            out[key] = sanitize(item, seen)
        end
    end

    seen[value] = nil
    return out
end

function M.encode(data, max_output: number): string
    local text, encode_err = json.encode(sanitize((data or {}) :: JsonValue))
    if encode_err then
        return json.encode({ error = "encode error", message = tostring(encode_err) })
    end
    if #text > max_output then
        return json.encode({
            truncated = true,
            original_length = #text,
            json_preview = safe_prefix(text, max_output),
        })
    end
    return text
end

M.sanitize = sanitize
M.safe_prefix = safe_prefix

return M
