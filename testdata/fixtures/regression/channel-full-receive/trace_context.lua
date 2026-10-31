local M = {}

local counter = 0

local function trim(value)
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function encode(value)
    return tostring(value):gsub("([^A-Za-z0-9_.~-])", function(ch)
        return string.format("%%%02X", string.byte(ch))
    end)
end

function M.pack_correlation(kind, parts)
    local k = trim(kind)
    if k == "" then return nil, "correlation kind is required" end
    local token = encode(k)
    if type(parts) == "table" then
        for _, part in ipairs(parts) do
            if part ~= nil then token = token .. ":" .. encode(part) end
        end
    elseif parts ~= nil then
        token = token .. ":" .. encode(parts)
    end
    return token, nil
end

function M.normalize(raw)
    if raw == nil then return nil, nil end
    if type(raw) ~= "table" then return nil, "trace_context must be an object" end
    local trace_id = trim(raw.trace_id)
    if trace_id == "" then return nil, "trace_context.trace_id is required" end
    local out = { trace_id = trace_id, run_id = trim(raw.run_id) ~= "" and trim(raw.run_id) or trace_id }
    if trim(raw.caused_by_event_id) ~= "" then out.caused_by_event_id = trim(raw.caused_by_event_id) end
    if trim(raw.correlation_key) ~= "" then out.correlation_key = trim(raw.correlation_key) end
    return out, nil
end

function M.begin(_kind, correlation_key)
    counter = counter + 1
    local id = "trace-" .. tostring(counter)
    local out = { trace_id = id, run_id = id }
    if trim(correlation_key) ~= "" then out.correlation_key = trim(correlation_key) end
    return out
end

function M.common(items)
    if type(items) ~= "table" then return nil end
    for _, item in ipairs(items) do
        if type(item) == "table" and type(item.trace_context) == "table" then
            return item.trace_context
        end
        if type(item) == "table" and type(item.trace_id) == "string" then
            return item
        end
    end
    return nil
end

return M

