local json = require("json")
local consts = require("consts")
local workflow_state = {}
local methods = {}
local workflow_state_mt = { __index = methods }

local function normalize_node_config(config: any): any
    if type(config) == "table" then
        return config
    end

    if type(config) == "string" and config ~= "" then
        local parsed, parse_err = json.decode(config)
        if not parse_err and type(parsed) == "table" then
            return parsed
        end
    end

    return nil
end
function workflow_state.new(dataflow_id, options)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Dataflow ID is required"
    end

    local instance = {
        dataflow_id = dataflow_id,
        actor_id = nil :: string?,
        options = options or {},

        nodes = {},
        dataflow_metadata = {},
        loaded = false,

        active_processes = {},
        active_yields = {},
        pending_signal_wake_keys = {},

        input_tracker = {
            requirements = {},
            available = {}
        },

        has_workflow_output = false,
        has_workflow_error = false,

        queued_commands = {}
    }

    return setmetatable(instance, workflow_state_mt), nil
end
function methods:_update_state_from_results(results)
    if not results or not results.results then
        return
    end

    for _, result in ipairs(results.results) do
        if not result or not result.input then
            goto continue
        end

        local command = result.input
        local command_type = command.type
        local payload = command.payload or {}

        if command_type == consts.COMMAND_TYPES.CREATE_NODE and (result.node_id or payload.node_id) then
            local created_node_id = result.node_id or payload.node_id
            local config = normalize_node_config(payload.config)

            local node = {
                status = payload.status or consts.STATUS.PENDING,
                type = payload.node_type,
                parent_node_id = payload.parent_node_id,
                metadata = payload.metadata or {},
                config = config
            }
            self.nodes[created_node_id] = node

            self:_set_input_requirements_from_config(created_node_id, config)

        elseif command_type == consts.COMMAND_TYPES.UPDATE_NODE and payload.node_id then
            local node_id = payload.node_id
            local node = self.nodes[node_id]

            if node then
                if payload.node_type then
                    node.type = payload.node_type
                end
                if payload.status then
                    node.status = payload.status
                end
                if payload.metadata then
                    node.metadata = payload.metadata
                end
                if payload.config then
                    node.config = normalize_node_config(payload.config)
                    self:_set_input_requirements_from_config(node_id, node.config)
                end
            end
        elseif command_type == consts.COMMAND_TYPES.DELETE_NODE and payload.node_id then
            self.nodes[payload.node_id] = nil

        elseif command_type == consts.COMMAND_TYPES.UPDATE_WORKFLOW then
            if payload.metadata then
                for k, v in pairs(payload.metadata) do
                    self.dataflow_metadata[k] = v
                end
            end

        elseif command_type == consts.COMMAND_TYPES.CREATE_DATA then
            if payload.data_type == consts.DATA_TYPE.WORKFLOW_OUTPUT then
                if payload.discriminator == "error" then
                    self.has_workflow_error = true
                else
                    self.has_workflow_output = true
                end
            elseif payload.data_type == consts.DATA_TYPE.NODE_INPUT and payload.node_id then
                if not self.input_tracker.available[payload.node_id] then
                    self.input_tracker.available[payload.node_id] = {}
                end
                local key = payload.discriminator or payload.key or "default"
                self.input_tracker.available[payload.node_id][key] = true
            elseif payload.data_type == consts.DATA_TYPE.NODE_SIGNAL then
                -- deliver signal data to the matching waiting yield
                local signal_id = payload.key or payload.discriminator
                local signal_wake_key = type(payload.data_id) == "string" and
                    ("signal:" .. payload.data_id) or nil
                if signal_wake_key then self.pending_signal_wake_keys[signal_wake_key] = true end
                if signal_id then
                    for node_id, yield_info in pairs(self.active_yields) do
                        if yield_info.wait_for_signal and yield_info.signal_id == signal_id then
                            yield_info.signal_data = payload.content
                            if type(payload.data_id) == "string" then
                                yield_info.signal_wake_key = "signal:" .. payload.data_id
                                yield_info.signal_wake_keys = yield_info.signal_wake_keys or {}
                                table.insert(yield_info.signal_wake_keys, yield_info.signal_wake_key)
                                self.pending_signal_wake_keys[yield_info.signal_wake_key] = nil
                            end
                            break
                        end
                    end
                end
            end
        end

        ::continue::
    end
end

return workflow_state
