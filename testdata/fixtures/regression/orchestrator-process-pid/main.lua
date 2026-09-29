local uuid = require("uuid")
local json = require("json")
local consts = require("consts")
local security = require("security")

local orchestrator = {
    workflow_state = require("workflow_state"),
    scheduler = require("scheduler"),
    process = process,
    funcs = require("funcs"),
    commit = require("commit"),
    security = security
}

local function workflow_actor(actor_id: string?, dataflow_id: string)
    if type(actor_id) ~= "string" or actor_id == "" then
        return nil
    end
    local current_actor = orchestrator.security.actor()
    if current_actor and current_actor:id() == actor_id then
        return current_actor
    end
    return orchestrator.security.new_actor(actor_id, {
        kind = "dataflow.workflow",
        dataflow_id = dataflow_id,
        source = "userspace.dataflow.runner:orchestrator"
    })
end

---Execute a single node
---@param state table Orchestrator state
---@param node_info table Node execution information
---@return string|nil error Error message if spawn failed
local function execute_single_node(state, node_info)
    local node_id = node_info.node_id
    local node_type = node_info.node_type
    local path = node_info.path or {}

    if type(node_type) ~= "string" or node_type == "" then
        return "Invalid node type for node: " .. tostring(node_id)
    end

    if state.active_processes[node_id] then
        return nil -- Already running, skip
    end

    local node_data = state.workflow_state:get_node(node_id)
    if not node_data then
        return "Node not found: " .. node_id
    end

    local spawner = orchestrator.process.with_context({})
    if state.actor then
        spawner = spawner:with_actor(state.actor)
    end

    local pid, err_spawn = spawner:spawn_linked_monitored(node_type, consts.HOST_ID, {
        dataflow_id = state.dataflow_id,
        node_id = node_id,
        node = node_data,
        path = path
    })

    if not pid then
        return "Failed to spawn node process for node: " .. node_id .. ". Reason: " .. tostring(err_spawn)
    end

    state.workflow_state:track_process(node_id, pid)
    state.active_processes[node_id] = { pid = pid, path = path }

    return nil
end

---Process pending commits immediately
---@param state table Orchestrator state
---@return boolean success Whether processing succeeded
local function process_pending_commits(state)
    if #state.incoming_commit_queue == 0 then
        return true
    end

    -- Find new commits to process
    local commits_to_process = {}
    for _, commit_id in ipairs(state.incoming_commit_queue) do
        local already_processed = false
        for _, processed_id in ipairs(state.processed_commit_ids) do
            if processed_id == commit_id then
                already_processed = true
                break
            end
        end
        if not already_processed then
            table.insert(commits_to_process, commit_id)
        end
    end

    if #commits_to_process == 0 then
        return true
    end

    local result, err = state.workflow_state:process_commits(commits_to_process)
    if err then
        state.workflow_state:queue_commands({
            type = consts.COMMAND_TYPES.UPDATE_WORKFLOW,
            payload = {
                status = consts.STATUS.COMPLETED_FAILURE,
                metadata = { error = "Commit processing failed: " .. err }
            }
        })
        local persist_result, persist_err = state.workflow_state:persist()
        state.exit_result = {
            success = false,
            dataflow_id = state.dataflow_id,
            error = "Commit processing failed: " .. err
        }
        state.running = false
        return false
    end

    for _, commit_id in ipairs(commits_to_process) do
        table.insert(state.processed_commit_ids, commit_id)
    end

    return true
end

---Load pending commits from durable storage for crash/restart recovery
---@param state table Orchestrator state
---@return boolean success Whether loading succeeded
local function load_startup_pending_commits(state)
    local pending_commit_ids, pending_err = orchestrator.commit.get_pending_commits(state.dataflow_id)
    if pending_err then
        local failure_message = "Failed to load pending commits: " .. pending_err
        state.workflow_state:queue_commands({
            type = consts.COMMAND_TYPES.UPDATE_WORKFLOW,
            payload = {
                status = consts.STATUS.COMPLETED_FAILURE,
                metadata = { error = failure_message }
            }
        })
        local _persist_result, _persist_err = state.workflow_state:persist()
        state.exit_result = {
            success = false,
            dataflow_id = state.dataflow_id,
            error = failure_message
        }
        state.running = false
        return false
    end

    for _, commit_id in ipairs(pending_commit_ids or {}) do
        table.insert(state.incoming_commit_queue, commit_id)
    end

    return true
end

---Call scheduler and handle the result immediately
---@param state table Orchestrator state
---@return boolean continue Whether to continue processing
local function call_scheduler_and_handle(state)
    -- loop through SATISFY_YIELD decisions: they mutate state (clear active_yields)
    -- but don't guarantee forward progress on their own, especially when the yield's
    -- parent process is dead (recovery case). keep scheduling until a node starts,
    -- the workflow completes, or no more work can be dispatched.
    local max_iterations = 64
    while max_iterations > 0 do
        max_iterations = max_iterations - 1

        local snapshot = state.workflow_state:get_scheduler_snapshot()
        local decision = orchestrator.scheduler.find_next_work(snapshot)

        if decision.type == orchestrator.scheduler.DECISION_TYPE.EXECUTE_NODES then
            return handle_execute_nodes(state, decision.payload)
        elseif decision.type == orchestrator.scheduler.DECISION_TYPE.COMPLETE_WORKFLOW then
            return handle_complete_workflow(state, decision.payload)
        elseif decision.type == orchestrator.scheduler.DECISION_TYPE.SATISFY_YIELD then
            local cont = handle_satisfy_yield(state, decision.payload)
            if not cont or not state.running then
                return cont
            end
            -- re-enter the loop: yield satisfied, state changed, re-schedule
        else
            return true
        end
    end

    return true
end

---Handle node execution immediately
---@param state table Orchestrator state
---@param payload table Execution payload
---@return boolean continue Whether to continue processing
function handle_execute_nodes(state, payload)
    local nodes = payload.nodes or {}

    if #nodes == 0 then
        return true
    end

    -- Filter out already running nodes
    local nodes_to_execute = {}
    for _, node_info in ipairs(nodes) do
        local node_id = node_info.node_id
        if not state.active_processes[node_id] then
            table.insert(nodes_to_execute, node_info)
        end
    end

    if #nodes_to_execute == 0 then
        return true
    end

    -- Update all nodes to RUNNING status first
    local commands = {}
    for _, node_info in ipairs(nodes_to_execute) do
        table.insert(commands, {
            type = consts.COMMAND_TYPES.UPDATE_NODE,
            payload = {
                node_id = node_info.node_id,
                status = consts.STATUS.RUNNING
            }
        })
    end

    -- Update workflow status if needed
    if not state.workflow_status_updated then
        table.insert(commands, {
            type = consts.COMMAND_TYPES.UPDATE_WORKFLOW,
            payload = {
                status = consts.STATUS.RUNNING
            }
        })
        state.workflow_status_updated = true
    end

    state.workflow_state:queue_commands(commands)
    local result, err = state.workflow_state:persist()
    if err then
        local fail_msg = "Failed to persist RUNNING status for nodes: " .. err
        local fail_commands = {}
        for _, node_info in ipairs(nodes_to_execute) do
            table.insert(fail_commands, {
                type = consts.COMMAND_TYPES.UPDATE_NODE,
                payload = {
                    node_id = node_info.node_id,
                    status = consts.STATUS.COMPLETED_FAILURE,
                    metadata = { error = fail_msg }
                }
            })
        end
        table.insert(fail_commands, {
            type = consts.COMMAND_TYPES.UPDATE_WORKFLOW,
            payload = {
                status = consts.STATUS.COMPLETED_FAILURE,
                metadata = { error = fail_msg }
            }
        })
        state.workflow_state:queue_commands(fail_commands)
        local persist_result, persist_err = state.workflow_state:persist()
        state.exit_result = {
            success = false,
            dataflow_id = state.dataflow_id,
            error = fail_msg
        }
        state.running = false
        return false
    end

    -- Spawn processes
    local execution_failures = {}
    for _, node_info in ipairs(nodes_to_execute) do
        local spawn_err = execute_single_node(state, node_info)
        if spawn_err then
            table.insert(execution_failures, {
                node_id = node_info.node_id,
                error = spawn_err
            })
        end
    end

    -- Handle any spawn failures
    if #execution_failures > 0 then
        local fail_commands = {}
        local error_messages = {}

        for _, failure in ipairs(execution_failures) do
            table.insert(fail_commands, {
                type = consts.COMMAND_TYPES.UPDATE_NODE,
                payload = {
                    node_id = failure.node_id,
                    status = consts.STATUS.COMPLETED_FAILURE,
                    metadata = { error = failure.error }
                }
            })
            table.insert(error_messages, failure.node_id .. ": " .. failure.error)
        end

        local combined_error = "Node spawn failures: " .. table.concat(error_messages, "; ")
        table.insert(fail_commands, {
            type = consts.COMMAND_TYPES.UPDATE_WORKFLOW,
            payload = {
                status = consts.STATUS.COMPLETED_FAILURE,
                metadata = { error = combined_error }
            }
        })

        state.workflow_state:queue_commands(fail_commands)
        local persist_result, persist_err = state.workflow_state:persist()
        state.exit_result = {
            success = false,
            dataflow_id = state.dataflow_id,
            error = combined_error
        }
        state.running = false
        return false
    end

    return true
end

---Handle yield satisfaction immediately
---@param state table Orchestrator state
---@param payload table Yield payload
---@return boolean continue Whether to continue processing
function handle_satisfy_yield(state, payload)
    local parent_id = payload.parent_id
    local yield_id = payload.yield_id
    local reply_to = payload.reply_to
    local results = payload.results or {}
    if type(parent_id) ~= "string" then
        return true
    end
    if type(results) ~= "table" then
        results = {}
    end

    -- Queue yield satisfaction commands
    state.workflow_state:satisfy_yield(parent_id, results)

    -- Persist queued commands BEFORE sending reply
    local persist_result, persist_err = state.workflow_state:persist()
    if persist_err then
        return true
    end

    -- Send reply to yielding process ONLY AFTER successful persistence
    local process_info = state.active_processes[parent_id]
    if process_info and type(reply_to) == "string" then
        orchestrator.process.send(process_info.pid, reply_to, {
            yield_id = yield_id,
            response_data = {
                ok = true,
                run_node_results = results,
                all_completed = true
            }
        })
    end

    return true
end

---Handle workflow completion immediately
---@param state table Orchestrator state
---@param payload table Completion payload
---@return boolean continue Whether to continue processing (always false)
function handle_complete_workflow(state, payload)
    local success = payload.success
    local message = payload.message
    local final_status = success and consts.STATUS.COMPLETED_SUCCESS or consts.STATUS.COMPLETED_FAILURE

    -- If workflow failed, get detailed node error information
    local detailed_error = message
    if not success then
        local failed_node_errors = state.workflow_state:get_failed_node_errors()
        if failed_node_errors then
            detailed_error = failed_node_errors
        elseif not message then
            detailed_error = "Workflow failed"
        end
    end

    local commands = {
        {
            type = consts.COMMAND_TYPES.UPDATE_WORKFLOW,
            payload = {
                status = final_status,
                metadata = { error = not success and detailed_error or nil }
            }
        }
    }

    state.workflow_state:queue_commands(commands)
    local persist_result, persist_err = state.workflow_state:persist()

    if success then
        state.exit_result = {
            success = true,
            dataflow_id = state.dataflow_id,
            output = { message = message or "Workflow completed successfully" }
        }
    else
        state.exit_result = {
            success = false,
            dataflow_id = state.dataflow_id,
            error = detailed_error or "Workflow failed"
        }
    end

    state.running = false
    return false
end

---Handle yield request immediately
---@param state table Orchestrator state
---@param msg_payload table Yield request payload
---@param from_pid string Process ID that sent the request
local function handle_yield_request(state, msg_payload, from_pid)
    local node_id = nil
    local current_path = nil
    for nid, process_info in pairs(state.active_processes) do
        if process_info.pid == from_pid then
            node_id = nid
            current_path = process_info.path or {}
            break
        end
    end

    if not node_id then
        return
    end

    local yield_id = msg_payload and msg_payload.request_context and msg_payload.request_context.yield_id
    local yield_context = msg_payload and msg_payload.yield_context or {}
    local run_nodes = yield_context.run_nodes or {}
    if type(run_nodes) ~= "table" then
        run_nodes = {}
    end

    if #run_nodes == 0 then
        local wait_for_signal = yield_context.wait_for_signal
        local reply_to = msg_payload and msg_payload.request_context and msg_payload.request_context.reply_to

        if wait_for_signal then
            -- signal yield: track the yield and wait for external SATISFY_SIGNAL commit
            local yield_info = {
                yield_id = yield_id,
                reply_to = reply_to,
                signal_id = yield_context.signal_id or yield_id,
                pending_children = {},
                results = {},
                wait_for_signal = true,
            }
            state.workflow_state:track_yield(node_id, yield_info)
        elseif type(reply_to) == "string" and yield_id then
            orchestrator.process.send(from_pid, reply_to, {
                yield_id = yield_id,
                response_data = {
                    ok = true,
                    run_node_results = {},
                    all_completed = true
                }
            })
        end
    else
        local child_path = {}
        for _, ancestor_id in ipairs(current_path) do
            table.insert(child_path, ancestor_id)
        end
        table.insert(child_path, node_id)

        local yield_info = {
            yield_id = yield_id,
            reply_to = msg_payload and msg_payload.request_context and msg_payload.request_context.reply_to,
            pending_children = {},
            results = {},
            child_path = child_path
        }

        -- Only track non-template nodes in pending_children
        for _, child_id in ipairs(run_nodes) do
            if type(child_id) == "string" then
                local child_node = state.workflow_state:get_node(child_id)
                if child_node and child_node.status ~= consts.STATUS.TEMPLATE then
                    yield_info.pending_children[child_id] = consts.STATUS.PENDING
                end
            end
        end

        state.workflow_state:track_yield(node_id, yield_info)
    end
end

---Handle process events immediately
---@param state table Orchestrator state
---@param event table Process event
---@return boolean continue Whether to continue processing
local function handle_process_event(state, event)
    if event.kind ~= orchestrator.process.event.EXIT and event.kind ~= orchestrator.process.event.LINK_DOWN then
        return true
    end

    local from_pid = event.from
    local node_id = nil

    for nid, process_info in pairs(state.active_processes) do
        if process_info.pid == from_pid then
            node_id = nid
            break
        end
    end

    if not node_id then
        return true
    end

    state.active_processes[node_id] = nil

    local success = false
    local error_message = "Unknown exit reason"
    local result_data = nil

    if event.kind == orchestrator.process.event.EXIT then
        if event.result then
            result_data = event.result.value

            if event.result.error then
                success = false
                error_message = tostring(event.result.error)
            elseif type(result_data) == "table" and result_data.success == false then
                success = false
                error_message = tostring(result_data.error or "Node returned {success=false}")
            else
                success = true
            end
        else
            success = true
        end
    elseif event.kind == orchestrator.process.event.LINK_DOWN then
        success = false
        error_message = "Node process linked down"
    end

    local exit_info = state.workflow_state:handle_process_exit(from_pid, success, result_data)

    local persist_result, persist_err = state.workflow_state:persist()

    if exit_info and exit_info.yield_complete then
        return handle_satisfy_yield(state, {
            parent_id = exit_info.yield_complete.parent_id,
            yield_id = exit_info.yield_complete.yield_info.yield_id,
            reply_to = exit_info.yield_complete.yield_info.reply_to,
            results = exit_info.yield_complete.yield_info.results
        })
    end

    return true
end

---Handle commit message immediately
---@param state table Orchestrator state
---@param msg_payload table Commit payload
local function handle_commit_message(state, msg_payload)
    local commit_id = msg_payload and msg_payload.commit_id
    if commit_id then
        table.insert(state.incoming_commit_queue, commit_id)
    end
end

---Handle cancellation request
---@param state table Orchestrator state
---@param event table Cancel event
local function handle_cancellation(state, event)
    for node_id, process_info in pairs(state.active_processes) do
        if type(process_info.pid) == "string" then
            orchestrator.process.terminate(process_info.pid)
        end
    end

    state.workflow_state:queue_commands({
        type = consts.COMMAND_TYPES.UPDATE_WORKFLOW,
        payload = {
            status = consts.STATUS.CANCELLED,
            metadata = { cancellation_reason = "Received cancellation request" }
        }
    })
    local persist_result, persist_err = state.workflow_state:persist()

    state.exit_result = {
        success = false,
        dataflow_id = state.dataflow_id,
        error = "Workflow cancelled by request"
    }
    state.running = false
end

---Main orchestrator function
---@param args table Arguments containing dataflow_id and optional init_func_id
---@return table result Orchestration result with success/error
local function run(args)
    local dataflow_id_raw = args and args.dataflow_id
    local init_func_id = args and args.init_func_id

    if type(dataflow_id_raw) ~= "string" or dataflow_id_raw == "" then
        return { success = false, error = "Missing required dataflow_id" }
    end
    local dataflow_id = dataflow_id_raw

    local ws, ws_err = orchestrator.workflow_state.new(dataflow_id)
    if ws_err then
        return { success = false, error = "Failed to create workflow state: " .. ws_err }
    end
    if not ws then
        return { success = false, dataflow_id = dataflow_id, error = "Failed to create workflow state" }
    end
    local workflow_state = ws :: any

    -- Initialize state
    local state = {
        dataflow_id = dataflow_id,
        workflow_state = workflow_state,
        active_processes = {},
        incoming_commit_queue = {},
        processed_commit_ids = {},
        workflow_status_updated = false,
        running = true,
        exit_result = nil
    }

    -- Register process — if another orchestrator is already running, exit
    local _, reg_err = orchestrator.process.registry.register("dataflow." .. dataflow_id)
    if reg_err then
        return {
            success = true,
            dataflow_id = dataflow_id,
            error = nil,
            message = "Another orchestrator is already running for this workflow"
        }
    end
    orchestrator.process.set_options({ trap_links = true })

    -- Load workflow state
    local result, load_err = workflow_state:load_state()
    if load_err then
        return {
            success = false,
            dataflow_id = dataflow_id,
            error = "Failed to load workflow state: " .. load_err
        }
    end

    -- Recover commit backlog that may have accumulated while orchestrator was offline.
    -- This must happen before empty-workflow detection because commits can create nodes.
    local backlog_loaded = load_startup_pending_commits(state)
    if not backlog_loaded then
        return state.exit_result or {
            success = false,
            dataflow_id = dataflow_id,
            error = "Failed to recover pending commits"
        }
    end

    local pending_processed = process_pending_commits(state)
    if not pending_processed then
        return state.exit_result or {
            success = false,
            dataflow_id = dataflow_id,
            error = "Failed to process pending commits"
        }
    end

    local raw_actor_id = workflow_state:get_actor_id()
    local actor_id: string? = nil
    if type(raw_actor_id) == "string" and raw_actor_id ~= "" then
        actor_id = raw_actor_id
    end
    state.actor = workflow_actor(actor_id, dataflow_id)

    -- Check for empty workflow after applying pending commits
    local nodes = workflow_state:get_nodes()
    local node_count = 0
    for _ in pairs(nodes) do
        node_count = node_count + 1
    end

    if node_count == 0 then
        return {
            success = true,
            dataflow_id = dataflow_id,
            output = { message = "Empty workflow - no nodes to execute" }
        }
    end

    -- Call init function if provided
    if type(init_func_id) == "string" and init_func_id ~= "" then
        local executor = orchestrator.funcs.new()
        if state.actor then
            executor = executor:with_actor(state.actor)
        end
        local _, _ = executor:call(init_func_id, {
            dataflow_id = dataflow_id,
            metadata = workflow_state:get_dataflow_metadata()
        })
    end

    -- Set up channels
    local inbox = orchestrator.process.inbox()
    local events = orchestrator.process.events()

    -- Initial scheduler call
    local continue = call_scheduler_and_handle(state)
    if not continue then
        return state.exit_result or {
            success = false,
            dataflow_id = dataflow_id,
            error = "Orchestrator exited without result"
        }
    end

    -- Main processing loop
    while state.running do
        local result = channel.select({
            inbox:case_receive(),
            events:case_receive()
        })

        if not result.ok then
            break
        end

        if result.channel == inbox then
            local msg = result.value
            local topic = msg:topic()
            local payload = msg:payload():data()
            local payload_table = nil
            if type(payload) == "table" then
                payload_table = payload
            end
            local from_pid = msg:from()

            if topic == consts.MESSAGE_TOPIC.COMMIT then
                handle_commit_message(state, payload_table)
                local success = process_pending_commits(state)
                if success and state.running then
                    call_scheduler_and_handle(state)
                end
            elseif topic == consts.MESSAGE_TOPIC.YIELD_REQUEST then
                -- Process pending commits FIRST, before ANY yield handling
                local success = process_pending_commits(state)
                if success and state.running then
                    handle_yield_request(state, payload_table, from_pid)
                    call_scheduler_and_handle(state)
                end
            end
        elseif result.channel == events then
            local event = result.value

            if event.kind == orchestrator.process.event.CANCEL then
                handle_cancellation(state, event)
            else
                local continue = handle_process_event(state, event)
                if continue and state.running then
                    -- load pending commits from DB before scheduling
                    -- exiting node may have submitted output data (commit in DB but message not yet received)
                    load_startup_pending_commits(state)
                    process_pending_commits(state)
                    call_scheduler_and_handle(state)
                end
            end
        end
    end

    -- Clean up and return result
    return state.exit_result or { success = true, dataflow_id = dataflow_id }
end

orchestrator.run = run
return orchestrator

