local sql = require("sql")
local json = require("json")
local uuid = require("uuid")
local time = require("time")
local commit = require("commit")
local dataflow_repo = require("dataflow_repo")
local data_reader = require("data_reader")
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

local function normalize_string_array(values)
    if type(values) ~= "table" then
        return {}
    end

    local result = {}
    for _, value in ipairs(values) do
        if type(value) == "string" then
            table.insert(result, value)
        end
    end

    return result
end

local function sort_data_rows_desc(rows)
    table.sort(rows, function(a, b)
        -- Recovery only uses this on dataflow_data rows. Those rows are ordered by
        -- UUID v7 data_ids (ops.CREATE_DATA defaults to uuid.v7(), and built-in
        -- producers explicitly do the same), so data_id is the durable "latest"
        -- ordering even when created_at ties at backend timestamp precision.
        return tostring(a.data_id or "") > tostring(b.data_id or "")
    end)

    return rows
end

local function decode_json_content(content: any): any
    if type(content) == "string" then
        local parsed, parse_err = json.decode(content)
        if not parse_err then
            return parsed
        end
        return nil
    end

    return content
end

local function get_db()
    local db, err = sql.get(consts.APP_DB)
    if err then
        return nil, "Failed to connect to database: " .. err
    end
    return db
end

local function collect_routing_targets(node_config)
    local targets = {}
    if not node_config then
        return targets
    end

    for _, field in ipairs({ "data_targets", "error_targets" }) do
        local configured_targets = node_config[field]
        if type(configured_targets) == "table" then
            for _, target in ipairs(configured_targets) do
                if type(target) == "table" and type(target.data_type) == "string" then
                    table.insert(targets, target)
                end
            end
        end
    end

    return targets
end

local function input_target_slot(target)
    return target.discriminator or target.key or "default"
end

local function make_restart_metadata(previous_status, extra)
    local metadata = {
        orchestrator_restarted_at = time.now():format(time.RFC3339NANO),
        previous_status_on_restart = previous_status
    }

    if type(extra) == "table" then
        for k, v in pairs(extra) do
            metadata[k] = v
        end
    end

    return metadata
end

function workflow_state.new(dataflow_id, options)
    if not dataflow_id or dataflow_id == "" then
        return nil, "Dataflow ID is required"
    end

    local instance = {
        dataflow_id = dataflow_id,
        actor_id = nil :: string?,
        actor_context = nil :: any,
        options = options or {},

        nodes = {},
        dataflow_metadata = {},
        loaded = false,

        active_processes = {},
        active_yields = {},
        satisfied_signal_yields = {},
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

function methods:_query_target_rows(producer_node_id, target)
    local target_node_id = target.node_id or producer_node_id
    local query = data_reader.with_dataflow(self.dataflow_id)
        :with_nodes(target_node_id)
        :with_data_types(target.data_type)

    if target.key ~= nil then
        query = query:with_data_keys(target.key)
    end

    if target.discriminator ~= nil then
        query = query:with_data_discriminators(target.discriminator)
    end

    return query:all()
end

function methods:_find_routed_data_matches(producer_node_id, targets)
    local matched_rows = {}
    local seen_ids = {}

    for _, target in ipairs(targets or {}) do
        local rows = self:_query_target_rows(producer_node_id, target)
        for _, row in ipairs(rows or {}) do
            if row.data_id and not seen_ids[row.data_id] then
                seen_ids[row.data_id] = true
                table.insert(matched_rows, row)
            end
        end
    end

    return matched_rows
end

function methods:_load_node_result_row(node_id)
    local result_rows = data_reader.with_dataflow(self.dataflow_id)
        :with_nodes(node_id)
        :with_data_types(consts.DATA_TYPE.NODE_RESULT)
        :all()

    if not result_rows or #result_rows == 0 then
        return nil
    end

    sort_data_rows_desc(result_rows)
    return result_rows[1]
end

function methods:_detect_running_node_recovery(node_id)
    local node_data = self.nodes[node_id]
    if not node_data then
        return nil
    end

    local result_row = self:_load_node_result_row(node_id)
    if result_row then
        local discriminator = tostring(result_row.discriminator or "")
        if discriminator == "result.error" then
            return {
                status = consts.STATUS.COMPLETED_FAILURE,
                result_row = result_row
            }
        end

        return {
            status = consts.STATUS.COMPLETED_SUCCESS,
            result_row = result_row
        }
    end

    local config = node_data.config or {}
    local error_rows = self:_find_routed_data_matches(node_id, config.error_targets)
    if #error_rows > 0 then
        return {
            status = consts.STATUS.COMPLETED_FAILURE,
            synthetic_result = {
                discriminator = "result.error",
                content = {
                    recovered = true,
                    message = "Recovered failed node from durable error output"
                }
            }
        }
    end

    local success_rows = self:_find_routed_data_matches(node_id, config.data_targets)
    if #success_rows > 0 then
        return {
            status = consts.STATUS.COMPLETED_SUCCESS,
            synthetic_result = {
                discriminator = "result.success",
                content = {
                    recovered = true,
                    message = "Recovered completed node from durable output"
                }
            }
        }
    end

    return nil
end

function methods:_queue_node_reset(commands, node_id, previous_status, extra_metadata)
    if not self._reset_node_ids[node_id] then
        self._reset_node_ids[node_id] = true
    end

    table.insert(commands, {
        type = consts.COMMAND_TYPES.UPDATE_NODE,
        payload = {
            node_id = node_id,
            status = consts.STATUS.PENDING,
            metadata = make_restart_metadata(previous_status, extra_metadata)
        }
    })

    if self.nodes[node_id] then
        self.nodes[node_id].status = consts.STATUS.PENDING
    end
end

function methods:_queue_recovered_completion(commands, node_id, recovery_info)
    table.insert(commands, {
        type = consts.COMMAND_TYPES.UPDATE_NODE,
        payload = {
            node_id = node_id,
            status = recovery_info.status,
            metadata = {
                recovered_from_restart = true,
                recovery_reason = "durable_completion_detected"
            }
        }
    })

    if recovery_info.synthetic_result then
        table.insert(commands, {
            type = consts.COMMAND_TYPES.CREATE_DATA,
            payload = {
                data_id = uuid.v7(),
                data_type = consts.DATA_TYPE.NODE_RESULT,
                content = recovery_info.synthetic_result.content,
                node_id = node_id,
                discriminator = recovery_info.synthetic_result.discriminator
            }
        })
    end

    if self.nodes[node_id] then
        self.nodes[node_id].status = recovery_info.status
    end
end

function methods:_queue_recovery_duplicate_cleanup(commands, node_id)
    local node_data = self.nodes[node_id]
    if not node_data then
        return
    end

    local deleted_data_ids = {}

    local function queue_duplicate_deletes(rows)
        if not rows or #rows <= 1 then
            return
        end

        sort_data_rows_desc(rows)
        for i = 2, #rows do
            local row = rows[i]
            if row and row.data_id and not deleted_data_ids[row.data_id] then
                deleted_data_ids[row.data_id] = true
                table.insert(commands, {
                    type = consts.COMMAND_TYPES.DELETE_DATA,
                    payload = {
                        data_id = row.data_id
                    }
                })
            end
        end
    end

    for _, target in ipairs(collect_routing_targets(node_data.config)) do
        queue_duplicate_deletes(self:_query_target_rows(node_id, target))
    end

    queue_duplicate_deletes(
        data_reader.with_dataflow(self.dataflow_id)
            :with_nodes(node_id)
            :with_data_types(consts.DATA_TYPE.NODE_RESULT)
            :all()
    )
end

function methods:_propagate_reset_to_dependents(commands)
    local queue = {}
    local queued = {}
    local deleted_data_ids = {}

    for node_id, _ in pairs(self._reset_node_ids or {}) do
        table.insert(queue, node_id)
        queued[node_id] = true
    end

    local queue_index = 1
    while queue_index <= #queue do
        local producer_node_id = queue[queue_index]
        queue_index = queue_index + 1

        local producer_node = self.nodes[producer_node_id]
        if not producer_node then
            goto continue
        end

        for _, target in ipairs(collect_routing_targets(producer_node.config)) do
            local rows = self:_query_target_rows(producer_node_id, target)
            for _, row in ipairs(rows or {}) do
                if row.data_id and not deleted_data_ids[row.data_id] then
                    deleted_data_ids[row.data_id] = true
                    table.insert(commands, {
                        type = consts.COMMAND_TYPES.DELETE_DATA,
                        payload = {
                            data_id = row.data_id
                        }
                    })
                end
            end

            if target.data_type == consts.DATA_TYPE.NODE_INPUT and type(target.node_id) == "string" then
                local consumer_node = self.nodes[target.node_id]
                local slot = input_target_slot(target)

                if self.input_tracker.available[target.node_id] then
                    self.input_tracker.available[target.node_id][slot] = nil
                end

                if consumer_node and consumer_node.status ~= consts.STATUS.TEMPLATE and
                   consumer_node.status ~= consts.STATUS.PENDING then
                    local previous_status = consumer_node.status
                    self:_queue_node_reset(commands, target.node_id, previous_status, {
                        reset_reason = "upstream_recovery",
                        reset_source_node_id = producer_node_id
                    })

                    if not queued[target.node_id] then
                        queued[target.node_id] = true
                        table.insert(queue, target.node_id)
                    end
                end
            end
        end

        ::continue::
    end
end

function methods:_prune_duplicate_routed_data()
    local cleanup_commands = {}

    for node_id, _ in pairs(self.nodes) do
        self:_queue_recovery_duplicate_cleanup(cleanup_commands, node_id)
    end

    if #cleanup_commands == 0 then
        return nil
    end

    local result, err = commit.execute(self.dataflow_id, uuid.v7(), cleanup_commands, { publish = false })
    if err then
        return "Failed to prune duplicate routed data: " .. err
    end

    self:_load_existing_data()
    return nil
end

function methods:_set_input_requirements_from_config(node_id, config)
    config = normalize_node_config(config)
    if not config then
        return
    end

    local inputs = config.inputs
    if inputs and type(inputs) == "table" then
        self:set_input_requirements(node_id, {
            required = normalize_string_array(inputs.required),
            optional = normalize_string_array(inputs.optional)
        })
    end
end

function methods:load_state()
    if self.loaded then
        return self, nil
    end

    local dataflow, err_df = dataflow_repo.get(self.dataflow_id)
    if err_df then
        return nil, "Failed to load dataflow: " .. err_df
    end

    if not dataflow then
        return nil, "Dataflow not found: " .. self.dataflow_id
    end

    self.actor_id = dataflow.actor_id
    self.actor_context = dataflow.actor_context
    self.dataflow_status = dataflow.status
    self.dataflow_metadata = dataflow.metadata or {}

    local nodes, err_nodes = dataflow_repo.get_nodes_for_dataflow(self.dataflow_id)
    if err_nodes then
        return nil, "Failed to load nodes: " .. err_nodes
    end

    self.nodes = {}
    for _, node in ipairs(nodes or {}) do
        local config = normalize_node_config(node.config)

        self.nodes[node.node_id] = {
            status = node.status,
            type = node.type,
            parent_node_id = node.parent_node_id,
            metadata = node.metadata or {},
            config = config
        }

        self:_set_input_requirements_from_config(node.node_id, config)
    end

    self:_load_existing_data()

    local reset_err = self:_reset_running_nodes()
    if reset_err then
        return nil, reset_err
    end

    self:_invalidate_inputs_from_reset_producers()

    local dedupe_err = self:_prune_duplicate_routed_data()
    if dedupe_err then
        return nil, dedupe_err
    end

    self:_reconstruct_active_yields()

    self.loaded = true
    return self, nil
end

function methods:_load_existing_data()
    self.has_workflow_output = false
    self.has_workflow_error = false

    local workflow_outputs = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.WORKFLOW_OUTPUT)
        :all()

    for _, output in ipairs(workflow_outputs) do
        if output.discriminator == "error" then
            self.has_workflow_error = true
        else
            self.has_workflow_output = true
        end
    end

    self:_update_input_availability()
end

function methods:_reset_running_nodes()
    local reset_commands = {}
    self._reset_node_ids = {}

    for node_id, node_data in pairs(self.nodes) do
        if node_data.status == consts.STATUS.RUNNING then
            local recovery_info = self:_detect_running_node_recovery(node_id)
            if recovery_info then
                self:_queue_recovered_completion(reset_commands, node_id, recovery_info)
                self:_queue_recovery_duplicate_cleanup(reset_commands, node_id)
            else
                self:_queue_node_reset(reset_commands, node_id, consts.STATUS.RUNNING)
            end
        end
    end

    self:_propagate_reset_to_dependents(reset_commands)

    if #reset_commands > 0 then
        local result, err = commit.execute(self.dataflow_id, uuid.v7(), reset_commands, { publish = false })
        if err then
            return "Failed to reset RUNNING nodes: " .. err
        end

        self:_load_existing_data()
    end

    return nil
end

function methods:_invalidate_inputs_from_reset_producers()
    -- _reset_running_nodes() already clears stale routed data and input slots
    -- for reset producers and all downstream dependents before reconstruction.
end

function methods:_reconstruct_active_yields()
    local yield_records = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.NODE_YIELD)
        :all()

    local yield_result_records = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.NODE_YIELD_RESULT)
        :fetch_options({
            metadata = false,
            resolve_references = false
        })
        :all()

    local armed_records = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.NODE_PARK_ARMED)
        :fetch_options({ content = false, metadata = false, resolve_references = false })
        :all()
    local signal_records = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.NODE_SIGNAL)
        :all()
    local consumed_signal_records = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.NODE_SIGNAL_CONSUMED)
        :fetch_options({ content = false, metadata = false, resolve_references = false })
        :all()
    local consumed_signal_ids = {}
    for _, consumed in ipairs(consumed_signal_records or {}) do
        local signal_data_id = consumed.key or consumed.discriminator
        if type(signal_data_id) == "string" then consumed_signal_ids[signal_data_id] = true end
    end
    for _, signal_record in ipairs(signal_records or {}) do
        local signal_data_id = tostring(signal_record.data_id or "")
        if signal_data_id ~= "" and not consumed_signal_ids[signal_data_id] then
            self.pending_signal_wake_keys["signal:" .. signal_data_id] = true
        end
    end
    local armed_yields = {}
    for _, armed in ipairs(armed_records or {}) do
        local armed_id = armed.key or armed.discriminator
        if type(armed_id) == "string" and armed_id ~= "" then armed_yields[armed_id] = true end
    end

    local satisfied_yields = {}
    for _, yield_result in ipairs(yield_result_records) do
        local yield_id = yield_result.key or yield_result.discriminator
        if type(yield_id) == "string" and yield_id ~= "" then
            satisfied_yields[yield_id] = yield_result
        end
    end

    sort_data_rows_desc(yield_records)
    local reconstructed_parents = {}

    for _, yield_record in ipairs(yield_records) do
        local parent_node_id = yield_record.node_id
        if type(parent_node_id) ~= "string" or parent_node_id == "" then
            goto continue
        end

        if reconstructed_parents[parent_node_id] then
            goto continue
        end

        local parent_node = self.nodes[parent_node_id]

        if not parent_node or (parent_node.status ~= consts.STATUS.PENDING and
            parent_node.status ~= consts.STATUS.WAITING) then
            reconstructed_parents[parent_node_id] = true
            goto continue
        end

        local yield_content = decode_json_content(yield_record.content)
        if not yield_content then
            goto continue
        end

        local yield_id = yield_content.yield_id
        if type(yield_id) ~= "string" or yield_id == "" then
            goto continue
        end

        local yield_context = yield_content.yield_context or {}
        local was_reset = self._reset_node_ids and self._reset_node_ids[parent_node_id]
        local satisfied_result = satisfied_yields[yield_id]
        -- A crash can happen after a signal result and its consumed marker are
        -- committed but before the resumed node persists its terminal result.
        -- Keep that resolved episode only for the reset node so its fresh
        -- mailbox can replay the durable result instead of arming a new wait
        -- for an already-consumed signal.
        local needs_signal_replay = yield_context.wait_for_signal == true and
            (was_reset or parent_node.status == consts.STATUS.WAITING)
        if satisfied_result and not needs_signal_replay then
            goto continue
        end

        local reply_to = yield_content.reply_to
        local run_nodes = yield_context.run_nodes or {}
        local child_path = yield_content.child_path or {}
        local pending_children = {}
        local results = {}

        for _, child_id in ipairs(run_nodes) do
            local child_node = self.nodes[child_id]
            if child_node and child_node.status ~= consts.STATUS.TEMPLATE then
                pending_children[child_id] = child_node.status

                if child_node.status == consts.STATUS.COMPLETED_SUCCESS or
                   child_node.status == consts.STATUS.COMPLETED_FAILURE then
                    local result_data = data_reader.with_dataflow(self.dataflow_id)
                        :with_nodes(child_id)
                        :with_data_types(consts.DATA_TYPE.NODE_RESULT)
                        :one()
                    if result_data then
                        results[child_id] = result_data.data_id
                    end
                end
            end
        end

        local yield_info = {
            yield_id = yield_id,
            episode_id = yield_id,
            wake_keys = { "yield:" .. yield_id },
            reply_to = reply_to,
            pending_children = pending_children,
            results = results,
            child_path = child_path,
            wait_for_signal = yield_context.wait_for_signal,
            signal_id = yield_context.signal_id,
            timeout = yield_context.timeout,
            timeout_ms = yield_context.timeout_ms,
            timeout_deadline = yield_context.timeout_deadline,
            park_ack = yield_context.park_ack == true,
            arm = yield_context.arm,
            arm_completed = armed_yields[yield_id] == true,
        }

        if satisfied_result then
            yield_info.signal_data = decode_json_content(satisfied_result.content)
            yield_info.wake_keys = {}
        end

        -- A parked node may have re-yielded several times while recovering.
        -- They are retries of one wait episode, not new logical waits. Rebuild
        -- the stable episode identity and every timer key so satisfaction
        -- consumes the whole retry set and the external arm key never changes.
        if yield_info.wait_for_signal then
            local episode_id = yield_id
            local wake_keys = {}
            local arm_completed = yield_info.arm_completed
            local episode_result = satisfied_result
            for _, candidate_record in ipairs(yield_records) do
                if candidate_record.node_id == parent_node_id then
                    local candidate = decode_json_content(candidate_record.content)
                    local candidate_id = candidate and candidate.yield_id
                    local candidate_context = candidate and candidate.yield_context or {}
                    if type(candidate_id) == "string" and candidate_id ~= "" and
                        candidate_context.wait_for_signal == true and
                        candidate_context.signal_id == yield_info.signal_id then
                        if candidate_id < episode_id then episode_id = candidate_id end
                        if armed_yields[candidate_id] == true then arm_completed = true end
                        local candidate_result = satisfied_yields[candidate_id]
                        if candidate_result then
                            episode_result = episode_result or candidate_result
                        else
                            table.insert(wake_keys, "yield:" .. candidate_id)
                        end
                    end
                end
            end
            yield_info.episode_id = episode_id
            yield_info.wake_keys = wake_keys
            yield_info.arm_completed = arm_completed
            if episode_result then
                -- A committed yield result is the continuation mailbox for the
                -- whole signal-wait episode. It wins over any later signal with
                -- the same public signal id and carries no signal wake key: the
                -- original signal was already consumed atomically.
                yield_info.signal_data = decode_json_content(episode_result.content)
                yield_info.signal_wake_keys = nil
            else
                for _, signal_record in ipairs(signal_records or {}) do
                    local signal_id = signal_record.key or signal_record.discriminator
                    if signal_id == yield_info.signal_id and not consumed_signal_ids[tostring(signal_record.data_id)] then
                        local content = decode_json_content(signal_record.content)
                        if content ~= nil then yield_info.signal_data = content end
                        local signal_wake_key = "signal:" .. tostring(signal_record.data_id)
                        yield_info.signal_wake_keys = yield_info.signal_wake_keys or {}
                        table.insert(yield_info.signal_wake_keys, signal_wake_key)
                        self.pending_signal_wake_keys[signal_wake_key] = nil
                    end
                end
            end
        end

        -- for reset nodes, the original yielding process is dead. the reply_to
        -- mailbox no longer exists; satisfying this yield would deliver data
        -- into a void AND trick the scheduler into thinking work is progressing.
        -- mark the yield as detached so the scheduler skips satisfaction and
        -- lets the parent re-run from scratch with a fresh reply_to. signal
        -- yields additionally keep signal_data in memory so a concurrently
        -- arriving signal is captured before the node re-attaches.
        if was_reset or parent_node.status == consts.STATUS.WAITING then
            yield_info.detached = true
        end

        self.active_yields[parent_node_id] = yield_info
        reconstructed_parents[parent_node_id] = true

        ::continue::
    end
end

function methods:_update_input_availability()
    for node_id, _ in pairs(self.input_tracker.requirements) do
        self.input_tracker.available[node_id] = {}
    end

    local node_inputs = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.NODE_INPUT)
        :all()

    for _, input in ipairs(node_inputs) do
        if input.node_id then
            if not self.input_tracker.available[input.node_id] then
                self.input_tracker.available[input.node_id] = {}
            end

            local key = input.discriminator or input.key or "default"
            self.input_tracker.available[input.node_id][key] = true
        end
    end
end

function methods:process_commits(commit_ids)
    if not commit_ids or #commit_ids == 0 then
        return { changes_made = false, message = "No commits to process" }, nil
    end

    for _, commit_id in ipairs(commit_ids) do
        table.insert(self.queued_commands, {
            type = consts.COMMAND.APPLY_COMMIT,
            payload = {
                commit_id = commit_id
            }
        })
    end

    local result, err = self:persist()
    if err then
        return nil, err
    end

    return result, nil
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
                                local matched_wake_key = "signal:" .. payload.data_id
                                yield_info.signal_wake_keys = yield_info.signal_wake_keys or {}
                                table.insert(yield_info.signal_wake_keys, matched_wake_key)
                                self.pending_signal_wake_keys[matched_wake_key] = nil
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

function methods:get_scheduler_snapshot()
    local active_proc_map = {}
    for node_id, pid in pairs(self.active_processes) do
        active_proc_map[node_id] = true
    end

    return {
        nodes = self.nodes,
        active_yields = self.active_yields,
        active_processes = active_proc_map,
        input_tracker = self.input_tracker,
        has_workflow_output = self.has_workflow_output,
        has_workflow_error = self.has_workflow_error
    }
end

function methods:get_failed_node_errors()
    local workflow_errors = data_reader.with_dataflow(self.dataflow_id)
        :with_data_types(consts.DATA_TYPE.WORKFLOW_OUTPUT)
        :all()

    for _, err_data in ipairs(workflow_errors) do
        if err_data.discriminator == "error" then
            local content = err_data.content
            if type(content) == "string" then
                return content
            elseif type(content) == "table" then
                return json.encode(content)
            else
                return tostring(content)
            end
        end
    end

    local failed_nodes = {}
    for node_id, node_data in pairs(self.nodes) do
        if node_data.status == consts.STATUS.COMPLETED_FAILURE then
            table.insert(failed_nodes, node_id)
        end
    end

    if #failed_nodes == 0 then
        return nil
    end

    local error_details = {}
    for _, node_id in ipairs(failed_nodes) do
        local result_data = data_reader.with_dataflow(self.dataflow_id)
            :with_nodes(node_id)
            :with_data_types(consts.DATA_TYPE.NODE_RESULT)
            :all()

        local error_message = "Unknown error"
        for _, result in ipairs(result_data) do
            if result.discriminator == "result.error" then
                local content = result.content or "Unknown error"

                if result.content_type == "application/json" or result.content_type == consts.CONTENT_TYPE.JSON then
                    local parsed, parse_err = json.decode(content)
                    if not parse_err and type(parsed) == "table" then
                        if parsed.error and type(parsed.error) == "table" and parsed.error.message then
                            error_message = tostring(parsed.error.message)
                        elseif parsed.message then
                            error_message = tostring(parsed.message)
                        else
                            error_message = tostring(content)
                        end
                    else
                        error_message = tostring(content)
                    end
                else
                    error_message = tostring(content)
                end
                break
            end
        end

        table.insert(error_details, "Node [" .. node_id .. "] failed: " .. error_message)
    end

    return table.concat(error_details, "; ")
end

function methods:_node_has_required_inputs(node_id)
    local requirements = self.input_tracker.requirements[node_id]
    if not requirements then
        local available = self.input_tracker.available[node_id] or {}
        return next(available) ~= nil
    end

    local available = self.input_tracker.available[node_id] or {}
    for _, required_key in ipairs(requirements.required or {}) do
        if not available[required_key] then
            return false
        end
    end

    return true
end

function methods:cancel_deadlocked_yield_children(parent_id, yield_info)
    if not yield_info or not yield_info.pending_children then
        return
    end

    local has_runnable_child = false
    for child_id, status in pairs(yield_info.pending_children) do
        if status == consts.STATUS.PENDING then
            local child_node = self.nodes[child_id]
            if child_node and self:_node_has_required_inputs(child_id) then
                has_runnable_child = true
                break
            end
        end
    end

    if not has_runnable_child then
        local cancel_commands = {}

        for child_id, status in pairs(yield_info.pending_children) do
            if status == consts.STATUS.PENDING then
                yield_info.pending_children[child_id] = consts.STATUS.CANCELLED
                table.insert(cancel_commands, {
                    type = consts.COMMAND_TYPES.UPDATE_NODE,
                    payload = {
                        node_id = child_id,
                        status = consts.STATUS.CANCELLED,
                        metadata = {
                            cancellation_reason = "Yield deadlock: no pending nodes can run"
                        }
                    }
                })

                if self.nodes[child_id] then
                    self.nodes[child_id].status = consts.STATUS.CANCELLED
                end
            end
        end

        if #cancel_commands > 0 then
            self:queue_commands(cancel_commands)
        end
    end
end

function methods:track_process(node_id, pid)
    self.active_processes[node_id] = pid
    return self
end

function methods:prepare_passivation(node_id)
    local yield_info = self.active_yields[node_id]
    if not yield_info then return nil, "active yield not found" end
    yield_info.detached = true
    if self.nodes[node_id] then self.nodes[node_id].status = consts.STATUS.WAITING end
    self:queue_commands({
        type = consts.COMMAND_TYPES.UPDATE_NODE,
        payload = { node_id = node_id, status = consts.STATUS.WAITING },
    })
    return true, nil
end

-- Release a node process after its indefinite signal wait has been persisted
-- and armed. The node remains pending and its active yield remains durable.
function methods:passivate_process(pid)
    for node_id, tracked_pid in pairs(self.active_processes) do
        if tracked_pid == pid then
            self.active_processes[node_id] = nil
            return node_id
        end
    end
    return nil
end

function methods:handle_process_exit(pid, success, result)
    local exited_node_id = nil
    for node_id, tracked_pid in pairs(self.active_processes) do
        if tracked_pid == pid then
            exited_node_id = node_id
            break
        end
    end

    if not exited_node_id then
        return nil
    end

    self.active_processes[exited_node_id] = nil
    self.satisfied_signal_yields[exited_node_id] = nil

    local new_status = success and consts.STATUS.COMPLETED_SUCCESS or consts.STATUS.COMPLETED_FAILURE
    if self.nodes[exited_node_id] then
        self.nodes[exited_node_id].status = new_status
    end

    local result_data_id = uuid.v7()
    local discriminator = success and "result.success" or "result.error"

    table.insert(self.queued_commands, {
        type = consts.COMMAND_TYPES.UPDATE_NODE,
        payload = {
            node_id = exited_node_id,
            status = new_status
        }
    })

    table.insert(self.queued_commands, {
        type = consts.COMMAND_TYPES.CREATE_DATA,
        payload = {
            data_id = result_data_id,
            data_type = consts.DATA_TYPE.NODE_RESULT,
            content = result or (success and "Completed" or "Failed"),
            node_id = exited_node_id,
            discriminator = discriminator
        }
    })

    local exit_info = {
        node_id = exited_node_id,
        success = success,
        result = result,
        result_data_id = result_data_id
    }

    -- Signal waits never have runnable child ownership, and a detached yield
    -- belongs to the pre-recovery process. If the resumed process exits before
    -- replacing that detached barrier, retaining it can relaunch the now-
    -- terminal parent forever. Attached child barriers remain until their live
    -- descendants drain through the normal parent/child exit path.
    local own_yield = self.active_yields[exited_node_id]
    if own_yield and (own_yield.wait_for_signal or own_yield.detached) then
        self.active_yields[exited_node_id] = nil
    end

    local node_data = self.nodes[exited_node_id]
    if node_data and node_data.parent_node_id then
        local parent_id = node_data.parent_node_id
        local yield_info = self.active_yields[parent_id]

        if yield_info and yield_info.pending_children and yield_info.pending_children[exited_node_id] then
            yield_info.pending_children[exited_node_id] = success and consts.STATUS.COMPLETED_SUCCESS or
            consts.STATUS.COMPLETED_FAILURE
            yield_info.results[exited_node_id] = result_data_id

            self:cancel_deadlocked_yield_children(parent_id, yield_info)

            local all_complete = true
            for child_id, status in pairs(yield_info.pending_children) do
                if status ~= consts.STATUS.COMPLETED_SUCCESS and
                   status ~= consts.STATUS.COMPLETED_FAILURE and
                   status ~= consts.STATUS.CANCELLED and
                   status ~= consts.STATUS.TERMINATED and
                   status ~= consts.STATUS.SKIPPED then
                    all_complete = false
                    break
                end
            end

            if all_complete then
                exit_info.yield_complete = {
                    parent_id = parent_id,
                    yield_info = yield_info
                }
            end
        end
    end

    return exit_info
end

function methods:track_yield(node_id, yield_info)
    if yield_info.wait_for_signal and yield_info.signal_id and not yield_info.signal_data then
        -- 1. Replay a result durably consumed by this orchestrator life. The
        -- reply can race process replacement: the old yield result is durable,
        -- but the replacement may re-yield before its predecessor's EXIT is
        -- observed. Reconstruction covers process restarts; this cache covers
        -- the same-owner handoff without requiring another external wake.
        local satisfied = self.satisfied_signal_yields[node_id]
        if satisfied and satisfied.signal_id == yield_info.signal_id then
            yield_info.signal_data = satisfied.signal_data
        end

        if yield_info.signal_data == nil then
            -- 2. inherit from detached yield (restart recovery: signal arrived while node restarting)
            local existing = self.active_yields[node_id]
            if existing and existing.detached and existing.signal_id == yield_info.signal_id and
                existing.signal_data ~= nil then
                yield_info.signal_data = existing.signal_data
                if type(existing.signal_wake_keys) == "table" then
                    yield_info.signal_wake_keys = {}
                    for _, wake_key in ipairs(existing.signal_wake_keys) do
                        table.insert(yield_info.signal_wake_keys, wake_key)
                        self.pending_signal_wake_keys[wake_key] = nil
                    end
                end
            else
                -- 3. check DB (pre-queued: signal arrived before node ever yielded)
                local consumed_records = data_reader.with_dataflow(self.dataflow_id)
                    :with_data_types(consts.DATA_TYPE.NODE_SIGNAL_CONSUMED)
                    :fetch_options({ content = false, metadata = false, resolve_references = false })
                    :all()
                local consumed_signal_ids = {}
                for _, consumed in ipairs(consumed_records or {}) do
                    local consumed_id = consumed.key or consumed.discriminator
                    if type(consumed_id) == "string" then consumed_signal_ids[consumed_id] = true end
                end
                local signal_records = data_reader.with_dataflow(self.dataflow_id)
                    :with_data_types(consts.DATA_TYPE.NODE_SIGNAL)
                    :all()
                for _, sig in ipairs(signal_records) do
                    local sig_key = sig.key or sig.discriminator
                    if sig_key == yield_info.signal_id and not consumed_signal_ids[tostring(sig.data_id)] then
                        -- DB content is raw bytes (string); parse JSON so scheduler
                        -- and handle_satisfy_yield see a table instead of coercing to {}
                        local content = sig.content
                        if type(content) == "string" then
                            local parsed, parse_err = json.decode(content)
                            if not parse_err then content = parsed end
                        end
                        yield_info.signal_data = content
                        local matched_wake_key = "signal:" .. tostring(sig.data_id)
                        yield_info.signal_wake_keys = yield_info.signal_wake_keys or {}
                        table.insert(yield_info.signal_wake_keys, matched_wake_key)
                        self.pending_signal_wake_keys[matched_wake_key] = nil
                    end
                end
            end
        end
    end

    self.active_yields[node_id] = yield_info
    return self
end

function methods:satisfy_yield(node_id, results)
    local yield_info = self.active_yields[node_id]
    if yield_info then
        local consume_wake_keys = {}
        if type(yield_info.wake_keys) == "table" then
            for _, wake_key in ipairs(yield_info.wake_keys) do table.insert(consume_wake_keys, wake_key) end
        else
            table.insert(consume_wake_keys, "yield:" .. tostring(yield_info.yield_id))
        end
        if type(yield_info.signal_wake_keys) == "table" then
            for _, wake_key in ipairs(yield_info.signal_wake_keys) do
                table.insert(consume_wake_keys, wake_key)
            end
        end
        table.insert(self.queued_commands, {
            type = consts.COMMAND_TYPES.CREATE_DATA,
            payload = {
                data_id = uuid.v7(),
                data_type = consts.DATA_TYPE.NODE_YIELD_RESULT,
                content = results,
                key = yield_info.yield_id,
                consume_wake_keys = consume_wake_keys,
                node_id = node_id
            }
        })
        for _, wake_key in ipairs(consume_wake_keys) do
            local retry_yield_id = string.match(wake_key, "^yield:(.+)$")
            if retry_yield_id and retry_yield_id ~= yield_info.yield_id then
                table.insert(self.queued_commands, {
                    type = consts.COMMAND_TYPES.CREATE_DATA,
                    payload = {
                        data_id = uuid.v7(),
                        data_type = consts.DATA_TYPE.NODE_YIELD_RESULT,
                        content = results,
                        key = retry_yield_id,
                        node_id = node_id,
                        consume_wake_keys = {},
                    },
                })
            end
            local signal_data_id = string.match(wake_key, "^signal:(.+)$")
            if signal_data_id then
                table.insert(self.queued_commands, {
                    type = consts.COMMAND_TYPES.CREATE_DATA,
                    payload = {
                        data_id = uuid.v7(),
                        data_type = consts.DATA_TYPE.NODE_SIGNAL_CONSUMED,
                        content = { consumed = true },
                        key = signal_data_id,
                        node_id = node_id,
                    },
                })
            end
        end

        if yield_info.wait_for_signal then
            self.satisfied_signal_yields[node_id] = {
                signal_id = yield_info.signal_id,
                signal_data = results,
            }
        end
        self.active_yields[node_id] = nil
    end

    return self
end

-- Stop tracking a parked wait whose external arm failed. The node process receives
-- the structured failure and persists its normal terminal node error; removing the
-- in-memory wait prevents a later signal from reviving the failed park.
function methods:abandon_yield(node_id)
    self.active_yields[node_id] = nil
    return self
end

function methods:observe_signal_wake(_wake_key)
    -- A wake message is only a hint that the durable commit may now be visible.
    -- Do not make its row eligible for unclaimed-signal cleanup until applying
    -- the corresponding NODE_SIGNAL record proves that the commit is durable.
    return self
end

function methods:take_unclaimed_signal_wake_keys()
    local keys = {}
    for wake_key in pairs(self.pending_signal_wake_keys) do table.insert(keys, wake_key) end
    self.pending_signal_wake_keys = {}
    return keys
end

function methods:set_input_requirements(node_id, requirements)
    self.input_tracker.requirements[node_id] = requirements

    if not self.input_tracker.available[node_id] then
        self.input_tracker.available[node_id] = {}
    end

    return self
end

function methods:queue_commands(commands)
    if type(commands) == "table" and commands.type then
        table.insert(self.queued_commands, commands)
    elseif type(commands) == "table" then
        for _, cmd in ipairs(commands) do
            table.insert(self.queued_commands, cmd)
        end
    end
    return self
end

-- A failed transaction leaves its command batch queued so callers may retry it.
-- Lifecycle failure handling deliberately replaces that batch with one fenced
-- terminal command; expose the reset as an explicit state operation rather than
-- reaching into the workflow-state instance from the orchestrator.
function methods:discard_queued_commands()
    self.queued_commands = {}
    return self
end

function methods:persist()
    if #self.queued_commands == 0 then
        return { changes_made = false, message = "No commands to persist" }, nil
    end

    local op_id = uuid.v7()
    local result, err = commit.execute(self.dataflow_id, op_id, self.queued_commands, { publish = true })

    if err then
        return nil, "Failed to persist commands: " .. err
    end

    self:_update_state_from_results(result)

    self.queued_commands = {}

    return result, nil
end

function methods:get_nodes()
    return self.nodes
end

function methods:get_node(node_id)
    return self.nodes[node_id]
end

function methods:get_node_result_data_id(node_id)
    local row = self:_load_node_result_row(node_id)
    return row and row.data_id or nil
end

function methods:get_dataflow_metadata()
    return self.dataflow_metadata
end

function methods:get_dataflow_status(): string?
    local status = self.dataflow_status
    if type(status) == "string" and status ~= "" then return status end
    return nil
end

function methods:get_actor_id(): string?
    local actor_id = self.actor_id
    if type(actor_id) == "string" and actor_id ~= "" then return actor_id end
    return nil
end

function methods:get_actor_context(): any
    return self.actor_context
end

function methods:is_node_active(node_id)
    if self.active_processes[node_id] then
        return true
    end

    if self.active_yields[node_id] then
        return true
    end

    for _, yield_info in pairs(self.active_yields) do
        if yield_info.pending_children and yield_info.pending_children[node_id] == consts.STATUS.PENDING then
            return true
        end
    end

    return false
end

return workflow_state
