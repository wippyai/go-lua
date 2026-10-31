local M = {}

local DEFAULT_LAZY_WAIT_SECONDS = 8
local DEFAULT_ASSESSOR_MODEL = "class:fast"
local DEFAULT_ASSESSOR_TIMEOUT_SECONDS = 30

type Map = { [string]: any }

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

function M.config(start: any): Map
    local cfg = type((start :: any).response_policy) == "table" and (start :: any).response_policy or {}
    return cfg :: Map
end

function M.lazy_enabled(start: any): boolean
    return trim(M.config(start).mode) == "lazy"
end

function M.should_assess(start: any, session_active: boolean): boolean
    return M.lazy_enabled(start) and session_active ~= true
end

function M.lazy_wait_ms(start: any): number
    local wait = tonumber(M.config(start).wait_seconds) or DEFAULT_LAZY_WAIT_SECONDS
    if wait < 1 then wait = 1 end
    if wait > 120 then wait = 120 end
    return math.floor(wait * 1000)
end

function M.assessor_model(start: any): string
    local model = trim(M.config(start).assessor_model)
    if model == "" then return DEFAULT_ASSESSOR_MODEL end
    return model
end

function M.assessor_timeout_seconds(start: any): number
    local timeout = tonumber(M.config(start).assessor_timeout_seconds) or DEFAULT_ASSESSOR_TIMEOUT_SECONDS
    if timeout < 1 then timeout = 1 end
    if timeout > 30 then timeout = 30 end
    return math.floor(timeout)
end

function M.assessor_options(start: any): Map
    return {
        model = M.assessor_model(start),
        temperature = 0,
        max_tokens = 8,
        timeout = tostring(M.assessor_timeout_seconds(start)) .. "s",
    }
end

function M.should_send_reply(start: any, content: string): boolean
    return trim(content) ~= ""
end

function M.channel_turn_text(system_prompt: any, body: string): string
    local blocks: { string } = {}
    local prompt_text = trim(system_prompt)
    if prompt_text ~= "" then
            blocks[#blocks + 1] = "Channel responder instructions:\n" .. prompt_text
    end
    blocks[#blocks + 1] = "Incoming channel message:\n" .. body
    return table.concat(blocks, "\n\n")
end

local function tool_label(function_name: any): string
    local name = trim(function_name)
    if name == "" or name == "Capabilities" then return "" end
    return (name:gsub("([a-z])([A-Z])", "%1 %2"):gsub("_", " "))
end

function M.tool_status_text(event_type: string, function_name: any): string?
    local label = tool_label(function_name)
    if label == "" then return nil end
    if event_type == "function_call" then
        return "Using " .. label .. "."
    end
    if event_type == "function_error" then
        return label .. " failed."
    end
    return nil
end

return M

