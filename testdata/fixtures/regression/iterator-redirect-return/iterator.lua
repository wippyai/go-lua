-- Registry source of userspace.dataflow.node.parallel:iterator, selected function.
local consts = require("consts")
local iterator = {}
local function build_iteration_discriminator(iteration_index, attempt_id)
    local padded_iteration = string.format("%06d", iteration_index)
    if type(attempt_id) == "string" and attempt_id ~= "" then
        return "iteration." .. padded_iteration .. "." .. attempt_id
    end
    return "iteration." .. padded_iteration
end
function iterator.redirect_terminals_to_parent(config, parent_node_id, iteration_index, source_node_id, attempt_id)
    local iteration_discriminator = build_iteration_discriminator(iteration_index, attempt_id)
    local new_data_targets = {}
    for target_index, target in ipairs(config.data_targets or {}) do
        if not target.node_id and target.data_type == consts.DATA_TYPE.NODE_OUTPUT then
            table.insert(new_data_targets, {
                data_type = consts.DATA_TYPE.ITERATION_RESULT,
                node_id = parent_node_id,
                discriminator = iteration_discriminator,
                key = source_node_id .. ":terminal:" .. tostring(target_index),
                content_type = target.content_type,
                metadata = {
                    source_node_id = source_node_id,
                    output_target_index = target_index,
                    terminal_emission_key_version = 1,
                    iteration = iteration_index,
                    attempt_id = attempt_id
                }
            })
        else
            table.insert(new_data_targets, target)
        end
    end

    local new_error_targets = {}
    for target_index, target in ipairs(config.error_targets or {}) do
        if not target.node_id and target.data_type == consts.DATA_TYPE.NODE_OUTPUT then
            table.insert(new_error_targets, {
                data_type = consts.DATA_TYPE.ITERATION_ERROR,
                node_id = parent_node_id,
                discriminator = iteration_discriminator,
                key = source_node_id .. ":terminal:" .. tostring(target_index),
                content_type = target.content_type,
                metadata = {
                    source_node_id = source_node_id,
                    output_key = target.key or "error",
                    output_target_index = target_index,
                    terminal_emission_key_version = 1,
                    iteration = iteration_index,
                    attempt_id = attempt_id
                }
            })
        else
            table.insert(new_error_targets, target)
        end
    end

    config.data_targets = new_data_targets
    config.error_targets = new_error_targets
    return config
end

return iterator
