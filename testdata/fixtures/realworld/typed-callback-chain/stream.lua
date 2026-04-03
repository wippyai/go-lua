local types = require("types")

local M = {}

function M.process(events: {any}, callbacks: StreamCallbacks?): (StreamResult?, string?)
    callbacks = callbacks or {}

    local on_content = callbacks.on_content
    local on_tool_call = callbacks.on_tool_call
    local on_error = callbacks.on_error
    local on_done = callbacks.on_done

    local result = types.empty_result()

    for _, event in ipairs(events) do
        if event.type == "content" then
            local chunk: string = event.data
            result.content = result.content .. chunk
            if on_content then
                on_content(chunk)
            end
        elseif event.type == "tool_call" then
            local call: ToolCall = {
                id = event.id,
                name = event.name,
                arguments = event.arguments or {},
            }
            table.insert(result.tool_calls, call)
            if on_tool_call then
                on_tool_call(call)
            end
        elseif event.type == "error" then
            local err: ErrorInfo = {message = event.message, code = event.code}
            if on_error then
                on_error(err)
            end
            return nil, err.message
        elseif event.type == "done" then
            result.finish_reason = event.reason
            result.usage = event.usage or result.usage
        end
    end

    if on_done then
        on_done(result)
    end

    return result, nil
end

return M
