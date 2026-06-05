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

    local root_label = json.decode_map("{}", protocol.node_type(), function(decoded)
        local decoded_id: string = decoded.id
        local bad_decoded_id: number = decoded.id -- expect-error
        return decoded_id
    end)
    local accepted_label: string = root_label
    local bad_label: number = root_label -- expect-error

    local row_labels = json.decode_many_map("[]", protocol.raw_record_array_type(), function(row)
        local row_amount: number = row.amount
        local bad_row_amount: string = row.amount -- expect-error
        return row.id .. tostring(row_amount)
    end)
    local accepted_labels: {string} = row_labels
    local bad_labels: {number} = row_labels -- expect-error

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

    local mapped_node_id = process.receive_map(node_ch, function(decoded)
        local decoded_id: string = decoded.id
        local bad_decoded_id: number = decoded.id -- expect-error
        return decoded_id
    end)
    if mapped_node_id then
        local mapped: string = mapped_node_id
        local bad_mapped: number = mapped_node_id -- expect-error
        id = id .. mapped
    end

    local mapped_node_summary = process.receive_map(node_ch, function(decoded)
        return {
            id = decoded.id,
            label = decoded.id .. ":node",
        }
    end)
    if mapped_node_summary then
        local summary_id: string = mapped_node_summary.id
        local summary_label: string = mapped_node_summary.label
        local bad_summary_id: number = mapped_node_summary.id -- expect-error
        local bad_summary_label: number = mapped_node_summary.label -- expect-error
        id = id .. summary_id .. summary_label
    end

    local mapped_node_stats = process.receive_map(node_ch, function(decoded)
        return {
            id = decoded.id,
            child_count = #decoded.children,
        }
    end)
    if mapped_node_stats then
        local stats_id: string = mapped_node_stats.id
        local child_count: number = mapped_node_stats.child_count
        local bad_stats_id: number = mapped_node_stats.id -- expect-error
        local bad_child_count: string = mapped_node_stats.child_count -- expect-error
        id = id .. stats_id .. tostring(child_count)
    end

    local tree = json.decode("{}", protocol.tree_type())
    if tree.kind == "group" then
        local first = tree.children[1]
        if first and first.kind == "text" then
            local value: string = first.value
            local bad_value: number = first.value -- expect-error
            id = id .. value
        end
    end
    if tree.kind == "text" then
        local children = tree.children -- expect-error
    end

    local tree_ch = process.listen("trees", {
        channel = source.trees,
        decode = protocol.tree_type(),
    })
    local tree_label = process.receive_map(tree_ch, function(decoded)
        if decoded.kind == "group" then
            local child = decoded.children[1]
            if child and child.kind == "text" then
                return child.value
            end
            return "empty"
        end
        return decoded.value
    end)
    if tree_label then
        local accepted_tree_label: string = tree_label
        local bad_tree_label: number = tree_label -- expect-error
        id = id .. accepted_tree_label
    end

    local wrong_ch: Channel<protocol.RawRecord> = process.listen("timers", { -- expect-error
        channel = source.timers,
        decode = protocol.timer_type(),
    })

    return id .. tostring(amount)
end

return handle
