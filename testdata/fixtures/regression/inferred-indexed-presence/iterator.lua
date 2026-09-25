local iterator = {}

function iterator.redirect_terminals_to_parent(config: any, _parent: string, _index: number, source_node_id: string, _attempt: string)
    local remapped = { data_targets = {} }
    for target_index, target in ipairs(config.data_targets or {}) do
        local remapped_target = {
            data_type = target.data_type,
            key = source_node_id .. ":terminal:" .. tostring(target_index),
        }
        table.insert(remapped.data_targets, remapped_target)
    end
    return remapped
end

return iterator
