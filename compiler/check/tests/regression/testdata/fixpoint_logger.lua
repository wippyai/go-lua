local function create_circular_buffer(size: integer)
    local buffer = table.create(size, 0)
    return {
        data = buffer,
        size = size,
        head = 1,
        count = 0,
    }
end

local function buffer_add(buf, item)
    buf.data[buf.head] = item
    buf.head = buf.head % buf.size + 1
    if buf.count < buf.size then
        buf.count = buf.count + 1
    end
end

local function buffer_get_all(buf)
    local result = table.create(buf.count, 0)
    if buf.count == 0 then
        return result
    end

    if buf.count < buf.size then
        for i = 1, buf.count do
            result[i] = buf.data[i]
        end
    else
        local idx = 1
        for i = buf.head, buf.size do
            result[idx] = buf.data[i]
            idx = idx + 1
        end
        for i = 1, buf.head - 1 do
            result[idx] = buf.data[i]
            idx = idx + 1
        end
    end

    return result
end

local function take_count(logs, count)
    if not count or count <= 0 or count >= #logs then
        return logs
    end

    local result = table.create(count, 0)
    for i = 1, count do
        result[i] = logs[i]
    end
    return result
end

local function handle_get_logs(state, payload)
    local logs = buffer_get_all(state.buffer)
    logs = take_count(logs, payload.count or 10)
    return { logs = logs, total = state.buffer.count }
end

local function handle_configure(state, payload)
    local new_buffer = create_circular_buffer(payload.buffer_size or 4)
    local existing = buffer_get_all(state.buffer)
    for i = 1, #existing do buffer_add(new_buffer, existing[i]) end
    state.buffer = new_buffer
end

local function flatten_log_entry(evt)
    local fields = {}
    for _, field in ipairs(evt.fields or {}) do
        fields[field.key] = field.value
    end
    return { level = evt.level, fields = fields }
end

local function run()
    local state = { buffer = create_circular_buffer(100), buffer_size = 100 }
    while true do
        local result = channel.select({})
        if not result.ok then break end
        if result.channel == "log" then
            local flattened = flatten_log_entry(result.value)
            buffer_add(state.buffer, flattened)
        elseif result.channel == "get" then
            handle_get_logs(state, result.value, "client")
        else
            handle_configure(state, result.value, "client")
        end
    end
    return { status = "completed" }
end

return { run = run }
