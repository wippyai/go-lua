local registry = require("registry")

-- Discovery for rasterizer plugins. A converter advertises that it can turn a
-- source mime type into page images by publishing a registry.entry with
-- meta.type = convert.rasterizer, carrying input_mime_types and a processor_func.
-- Adding a new rasterizer (DXF, TIFF, ...) is pure declaration; this resolver and
-- the pipeline stage never change. Mirrors upload_type.lua's find/match/priority.
local M = {}

local META_TYPE = "convert.rasterizer"

local function data(entry: any): { [string]: any }
    if type(entry) ~= "table" then return {} end
    if type(entry.data) == "table" then return entry.data :: { [string]: any } end
    return entry :: { [string]: any }
end

local function meta(entry: any): { [string]: any }
    if type(entry) == "table" and type(entry.meta) == "table" then return entry.meta :: { [string]: any } end
    return {}
end

local function priority(entry: any): number
    local d = data(entry)
    local m = meta(entry)
    return tonumber(d.priority or m.priority) or 0
end

local function mime_matches(entry: any, mime_type: string): boolean
    local d = data(entry)
    if type(d.input_mime_types) ~= "table" then return false end
    for _, mime in ipairs(d.input_mime_types) do
        if mime == mime_type or string.match(mime_type, mime :: string) then
            return true
        end
    end
    return false
end

-- find_by_mime(mime) -> rasterizer | nil. Highest-priority registered rasterizer
-- whose input_mime_types match the source mime, or nil when none can handle it.
function M.find_by_mime(mime_type: any): any?
    if type(mime_type) ~= "string" or mime_type == "" then return nil end

    local entries, err = registry.find({ [".kind"] = "registry.entry", ["meta.type"] = META_TYPE })
    if err or type(entries) ~= "table" then return nil end

    local matches: { any } = {}
    for _, entry in ipairs(entries) do
        if mime_matches(entry, mime_type) then matches[#matches + 1] = entry end
    end
    if #matches == 0 then return nil end

    table.sort(matches, function(a, b)
        local ap, bp = priority(a), priority(b)
        if ap ~= bp then return ap > bp end
        return tostring(a.id or "") < tostring(b.id or "")
    end)

    local top = matches[1]
    local d = data(top)
    local processor_func = d.processor_func
    if type(processor_func) ~= "string" or processor_func == "" then return nil end

    return {
        id = tostring(top.id or ""),
        processor_func = processor_func,
        input_mime_types = d.input_mime_types,
        priority = priority(top),
    }
end

return M

