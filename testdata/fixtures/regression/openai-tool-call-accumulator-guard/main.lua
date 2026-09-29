-- Reduced from wippy.llm.openai_compat:client. A tool-call id is inserted
-- before the same indexed entry is read later in the guarded block.
local function collect(deltas: {table})
    local tool_calls_accumulator = {}
    for _, tool_call_delta in ipairs(deltas) do
        local id = tool_call_delta.id
        if id and not tool_calls_accumulator[id] then
            tool_calls_accumulator[id] = {
                id = id,
                index = tool_call_delta.index,
                arguments = "",
                name = nil
            }
        elseif not id and tool_call_delta.index ~= nil then
            for tc_id, tc in pairs(tool_calls_accumulator) do
                if tc.index == tool_call_delta.index then
                    id = tc_id
                    break
                end
            end
        end

        if id then
            if tool_call_delta.index ~= nil then
                tool_calls_accumulator[id].index = tool_call_delta.index
            end
            if tool_call_delta["function"] then
                if tool_call_delta["function"].arguments then
                    tool_calls_accumulator[id].arguments =
                        (tool_calls_accumulator[id].arguments or "") ..
                        tool_call_delta["function"].arguments
                end
            end
            local tool_call = tool_calls_accumulator[id] :: any
            if tool_call.name and tool_call.arguments then
                print(tool_call.arguments)
            end
        end
    end
    return tool_calls_accumulator
end

local function direct_pairs()
    local acc = { a = { arguments = "" } }
    for key, _ in pairs(acc) do
        local id = key
        print(acc[id].arguments)
    end
    local id
    for key, _ in pairs(acc) do
        id = key
        break
    end
    if id then print(acc[id].arguments) end
end

local function guarded_insert(id: string?)
    local acc = {}
    if id and not acc[id] then
        acc[id] = { arguments = "" }
    end
    if id then print(acc[id].arguments) end
end

local function guarded_mutation(id: string?)
    local acc = {}
    if id and not acc[id] then
        acc[id] = { arguments = "", index = nil }
    end
    if id then
        acc[id].index = 1
        acc[id].arguments = acc[id].arguments .. "x"
    end
end

local function nested_guarded_mutation(id: string?, delta: table)
    local acc = {}
    if id and not acc[id] then
        acc[id] = { arguments = "", index = nil }
    end
    if id then
        if delta.index ~= nil then acc[id].index = delta.index end
        if delta["function"] and delta["function"].arguments then
            acc[id].arguments = (acc[id].arguments or "") .. delta["function"].arguments
        end
    end
end

return collect
