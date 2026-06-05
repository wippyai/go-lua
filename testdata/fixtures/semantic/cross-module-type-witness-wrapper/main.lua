local protocol = require("protocol")
local json = require("json")
local process = require("process")

local function handle(source: protocol.Source): string
    local record = json.decode("{}", protocol.raw_record_type())
    local id: string = record.id
    local bad_id: number = record.id -- expect-error
    local amount: number = record.amount
    local bad_amount: string = record.amount -- expect-error

    local rows = json.decode("[]", protocol.raw_record_array_type())
    local accepted_rows: {protocol.RawRecord} = rows
    local wrong_rows: {protocol.Timer} = rows -- expect-error

    for _, row in ipairs(rows) do
        local row_id: string = row.id
        local bad_row_id: number = row.id -- expect-error
        amount = amount + row.amount
    end

    local record_ch = process.listen("records", {
        channel = source.records,
        decode = protocol.raw_record_type(),
    })
    local received, ok = record_ch:receive()
    if ok then
        local received_id: string = received.id
        local bad_received_id: number = received.id -- expect-error
        id = id .. received_id
    end

    local timer_ch = process.listen("timers", {
        channel = source.timers,
        decode = protocol.timer_type(),
    })
    local timer, timer_ok = timer_ch:receive()
    if timer_ok then
        local elapsed: number = timer.elapsed
        local bad_elapsed: string = timer.elapsed -- expect-error
        amount = amount + elapsed
    end

    local nested_records = process.listen_nested("records", {
        channel = source.records,
        schema = {
            witness = protocol.raw_record_type(),
        },
    })
    local nested_record, nested_ok = nested_records:receive()
    if nested_ok then
        local nested_id: string = nested_record.id
        local bad_nested_id: number = nested_record.id -- expect-error
        amount = amount + nested_record.amount
    end

    local root = json.decode("{}", protocol.node_type())
    local root_id: string = root.id
    local bad_root_id: number = root.id -- expect-error
    for _, child in ipairs(root.children) do
        local child_id: string = child.id
        local bad_child_id: number = child.id -- expect-error
        id = id .. child_id
    end

    local node_ch = process.listen_nested("nodes", {
        channel = source.nodes,
        schema = {
            witness = protocol.node_type(),
        },
    })
    local node, node_ok = node_ch:receive()
    if node_ok then
        local node_id: string = node.id
        local bad_node_id: number = node.id -- expect-error
        for _, child in ipairs(node.children) do
            local child_id: string = child.id
            local bad_child_id: number = child.id -- expect-error
            id = id .. child_id
        end
    end

    local wrong_ch: Channel<protocol.RawRecord> = process.listen("timers", { -- expect-error
        channel = source.timers,
        decode = protocol.timer_type(),
    })

    return id .. tostring(amount)
end

return handle
