-- The storage boundary: every string the engine persists is valid UTF-8.
-- External content — scraped pages, model output — carries arbitrary bytes,
-- and Postgres rejects an invalid sequence, which would fail the whole commit
-- and strand the run. Invalid sequences are replaced with U+FFFD in place;
-- the record persists and the damage stays visible and local.
local M = {}

local REPLACEMENT = "\239\191\189" -- U+FFFD

local BINARY_CONTENT_TYPE = "application/octet-stream"

-- ensure_storable(value, content_type) -> value. The boundary is content-type
-- aware: declared binary passes through byte-identical — it lives in binary
-- columns and sanitizing it would be corruption. Everything else is text and
-- must be valid UTF-8.
function M.ensure_storable(value: any, content_type: any): any
    if content_type == BINARY_CONTENT_TYPE then return value end
    return M.ensure_utf8(value)
end

function M.ensure_utf8(s: any): any
    if type(s) ~= "string" then return s end
    local n = #s
    local i = 1
    local out: { string }? = nil
    local last = 1
    while i <= n do
        local c = string.byte(s, i)
        local len = 0
        if c < 0x80 then
            len = 1
        elseif c >= 0xC2 and c <= 0xDF then
            len = 2
        elseif c >= 0xE0 and c <= 0xEF then
            len = 3
        elseif c >= 0xF0 and c <= 0xF4 then
            len = 4
        end
        local ok = len > 0
        if ok and len > 1 then
            if i + len - 1 > n then
                ok = false
            else
                for j = 1, len - 1 do
                    local cc = string.byte(s, i + j)
                    if cc < 0x80 or cc > 0xBF then
                        ok = false
                        break
                    end
                end
                if ok and len == 3 then
                    local c2 = string.byte(s, i + 1)
                    if (c == 0xE0 and c2 < 0xA0) or (c == 0xED and c2 > 0x9F) then ok = false end
                elseif ok and len == 4 then
                    local c2 = string.byte(s, i + 1)
                    if (c == 0xF0 and c2 < 0x90) or (c == 0xF4 and c2 > 0x8F) then ok = false end
                end
            end
        end
        if ok then
            i = i + len
        else
            if out == nil then out = {} end
            local acc = out :: { string }
            acc[#acc + 1] = s:sub(last, i - 1)
            acc[#acc + 1] = REPLACEMENT
            i = i + 1
            last = i
        end
    end
    if out == nil then return s end
    local acc = out :: { string }
    acc[#acc + 1] = s:sub(last)
    return table.concat(acc)
end

return M

