-- kickside/converters/pdf2image/src/pdf2image/pdf2image.lua and the audit's
-- d-typeof-table repro: type(v) == "table" on a dynamic value yields a table
-- the source allows to be used as any map, list or record.
type Map = {[string]: any}

local function reject_unknown(options: { [string]: any }, allowed: { [string]: boolean }): string?
    for key in pairs(options) do
        if allowed[key] ~= true then
            return "unknown option: " .. tostring(key)
        end
    end
    return nil
end

local function normalize_options(opts: any, allowed: { [string]: boolean }): (any, string?)
    if opts == nil then
        return {}, nil
    end
    if type(opts) ~= "table" then
        return nil, "options must be a table, a dpi number, or nil"
    end
    local unknown = reject_unknown(opts, allowed)
    if unknown ~= nil then
        return nil, unknown
    end
    return opts, nil
end

local function as_map(v: any): Map
    if type(v) == "table" then return v end
    return {}
end

local function as_list(v: unknown): {any}
    if type(v) == "table" then return v end
    return {}
end

local function as_named(v: unknown): {name: string?}
    if type(v) == "table" then return v end
    return {}
end

local function not_a_string(v: any): string
    if type(v) == "table" then return v end -- expect-error: cannot return
    return ""
end

return { normalize_options = normalize_options, as_map = as_map, as_list = as_list, as_named = as_named, not_a_string = not_a_string }
