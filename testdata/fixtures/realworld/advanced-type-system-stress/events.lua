type MessageEvent = {kind: "message", id: string, text: string, tags: {string}?}
type ToolEvent = {kind: "tool", id: string, name: string, arguments: {[string]: any}}
type ErrorEvent = {kind: "error", id: string, error: {code: string, message: string}}
type Event = MessageEvent | ToolEvent | ErrorEvent

local M = {}
M.MessageEvent = MessageEvent
M.ToolEvent = ToolEvent
M.ErrorEvent = ErrorEvent
M.Event = Event

local function require_string(value, fallback: string): string
    if type(value) == "string" then
        return value
    end
    return fallback
end

local function string_array(value): {string}?
    if type(value) ~= "table" then
        return nil
    end
    local out: {string} = {}
    for _, item in ipairs(value) do
        if type(item) == "string" then
            table.insert(out, item)
        end
    end
    return out
end

function M.decode(raw: any): (Event?, string?)
    if type(raw) ~= "table" then
        return nil, "event must be a table"
    end

    if raw.kind == "message" then
        return {
            kind = "message",
            id = require_string(raw.id, ""),
            text = require_string(raw.text, ""),
            tags = string_array(raw.tags),
        }, nil
    end

    if raw.kind == "tool" then
        return {
            kind = "tool",
            id = require_string(raw.id, ""),
            name = require_string(raw.name, ""),
            arguments = type(raw.arguments) == "table" and (raw.arguments :: {[string]: any}) or {},
        }, nil
    end

    if raw.kind == "error" then
        return {
            kind = "error",
            id = require_string(raw.id, ""),
            error = {
                code = require_string(raw.code, "unknown"),
                message = require_string(raw.message, "failed"),
            },
        }, nil
    end

    return nil, "unknown event"
end

function M.render(event: Event): string
    if event.kind == "message" then
        return event.id .. ":" .. event.text
    end
    if event.kind == "tool" then
        return event.id .. ":" .. event.name
    end
    return event.id .. ":" .. event.error.code .. ":" .. event.error.message
end

function M.collect(raw_events: {any}): ({string}, {string})
    local rendered: {string} = {}
    local errors: {string} = {}
    for _, raw in ipairs(raw_events) do
        local event, err = M.decode(raw)
        if event then
            table.insert(rendered, M.render(event))
        else
            table.insert(errors, err or "unknown")
        end
    end
    return rendered, errors
end

return M
