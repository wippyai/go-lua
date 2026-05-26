package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

func TestLocalReplaceHarness_DecisionPayloadDefaultNarrowsByDiscriminant(t *testing.T) {
	source := `
local DECISION_TYPE = {
	SATISFY_YIELD = "satisfy_yield",
	COMPLETE_WORKFLOW = "complete_workflow",
}

local function create_decision(decision_type, payload)
	return {
		type = decision_type,
		payload = payload or {},
	}
end

local function find_next_work()
	return create_decision(DECISION_TYPE.SATISFY_YIELD, {
		parent_id = "parent",
		yield_id = "yield",
		reply_to = "pid",
		results = {},
	})
end

local function handle_satisfy_yield(payload)
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
	return yield_id ~= nil or reply_to ~= nil
end

local decision = find_next_work()
if decision.type == DECISION_TYPE.SATISFY_YIELD then
	handle_satisfy_yield(decision.payload)
end
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected discriminated decision payload to remain non-nil, got: %v", result.Errors)
	}
}

func TestLocalReplaceHarness_LocalReturnTableFieldFeedsCallArg(t *testing.T) {
	source := `
local function find_next_work()
	return {
		type = "satisfy_yield",
		payload = {
			parent_id = "parent",
		},
	}
end

local function handle(payload)
	return payload.parent_id
end

local decision = find_next_work()
if decision.type == "satisfy_yield" then
	handle(decision.payload)
end
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected local return table field to feed call arg, got: %v", result.Errors)
	}
}

func TestLocalReplaceHarness_RootLocalReturnInferenceWithoutStdlib(t *testing.T) {
	source := `
local function make_value()
	return {
		payload = {
			parent_id = "parent",
		},
	}
end

local function handle(payload)
	return payload.parent_id
end

local decision = make_value()
handle(decision.payload)
`

	result := testutil.Check(source)
	if result.HasError() {
		t.Fatalf("expected root local return inference without stdlib parent, got: %v", result.Errors)
	}
}

func TestLocalReplaceHarness_HeterogeneousConstantKeyHandlerTable(t *testing.T) {
	source := `
local COMMAND_TYPES = {
	CREATE_NODE = "create_node",
	UPDATE_NODE = "update_node",
}

local handlers = {}

handlers[COMMAND_TYPES.CREATE_NODE] = function(tx, dataflow_id, op_id, command)
	return {
		node_id = "node",
		changes_made = true,
		op_id = op_id,
	}, nil
end

handlers[COMMAND_TYPES.UPDATE_NODE] = function(tx, dataflow_id, op_id, command)
	return {
		node_id = "node",
		changes_made = false,
		op_id = op_id,
		message = "No fields provided for update",
	}, nil
end

local command = { type = COMMAND_TYPES.UPDATE_NODE }
local handler = handlers[command.type]
if not handler then
	error("missing handler")
end

local result, err = handler(nil, "flow", "op", command)
if err then
	error(err)
end
return result and result.changes_made
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected constant-key handler table to preserve callable join, got: %v", result.Errors)
	}
}

func TestLocalReplaceHarness_ImportedSchedulerDecisionPayload(t *testing.T) {
	scheduler := testutil.CheckAndExport(`
local scheduler = {}

local DECISION_TYPE = {
	EXECUTE_NODES = "execute_nodes",
	SATISFY_YIELD = "satisfy_yield",
	COMPLETE_WORKFLOW = "complete_workflow",
	NO_WORK = "no_work",
}

local function create_decision(decision_type, payload)
	return {
		type = decision_type,
		payload = payload or {},
	}
end

local function find_yield_driven_work(state)
	for parent_id, yield_info in pairs(state.active_yields) do
		return create_decision(DECISION_TYPE.SATISFY_YIELD, {
			parent_id = parent_id,
			yield_id = yield_info.yield_id,
			reply_to = yield_info.reply_to,
			results = yield_info.results or {},
		})
	end
	return nil
end

function scheduler.find_next_work(state)
	local decision = find_yield_driven_work(state)
	if decision then
		return decision
	end
	return create_decision(DECISION_TYPE.NO_WORK, {
		message = "No work available",
	})
end

scheduler.DECISION_TYPE = DECISION_TYPE

return scheduler
`, "scheduler", testutil.WithStdlib())
	if scheduler.HasError() {
		t.Fatalf("unexpected scheduler export errors: %v", testutil.ErrorMessages(scheduler.Errors))
	}

	encoded, err := io.EncodeManifest(scheduler.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	result := testutil.Check(`
local scheduler = require("scheduler")

local function handle_satisfy_yield(payload)
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
	return yield_id ~= nil or reply_to ~= nil
end

local decision = scheduler.find_next_work({
	active_yields = {
		parent = {
			yield_id = "yield",
			reply_to = "pid",
			results = {},
		},
	},
})

if decision.type == scheduler.DECISION_TYPE.SATISFY_YIELD then
	handle_satisfy_yield(decision.payload)
end
`,
		testutil.WithStdlib(),
		testutil.WithManifest("scheduler", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected imported scheduler payload to remain non-nil, got: %v", result.Errors)
	}
}

func TestLocalReplaceHarness_ImportedSchedulerSequentialDecisionProbes(t *testing.T) {
	scheduler := testutil.CheckAndExport(`
local scheduler = {}

local DECISION_TYPE = {
	EXECUTE_NODES = "execute_nodes",
	SATISFY_YIELD = "satisfy_yield",
	COMPLETE_WORKFLOW = "complete_workflow",
	NO_WORK = "no_work",
}

local function create_decision(decision_type, payload)
	return {
		type = decision_type,
		payload = payload or {},
	}
end

local function create_nodes_execution(nodes)
	return create_decision(DECISION_TYPE.EXECUTE_NODES, {
		nodes = nodes,
	})
end

local function find_yield_driven_work(state)
	for parent_id, yield_info in pairs(state.active_yields) do
		return create_decision(DECISION_TYPE.SATISFY_YIELD, {
			parent_id = parent_id,
			yield_id = yield_info.yield_id,
			reply_to = yield_info.reply_to,
			results = yield_info.results or {},
		})
	end
	return nil
end

local function find_input_ready_work(state)
	if state.ready then
		return create_nodes_execution({{node_id = "n", node_type = "worker"}})
	end
	return nil
end

local function check_workflow_completion(state)
	if state.done then
		return create_decision(DECISION_TYPE.COMPLETE_WORKFLOW, {
			success = true,
			message = "done",
		})
	end
	return nil
end

function scheduler.find_next_work(state)
	local decision = find_yield_driven_work(state)
	if decision then
		return decision
	end

	decision = find_input_ready_work(state)
	if decision then
		return decision
	end

	decision = check_workflow_completion(state)
	if decision then
		return decision
	end

	return create_decision(DECISION_TYPE.NO_WORK, {
		message = "No work available",
	})
end

scheduler.DECISION_TYPE = DECISION_TYPE

return scheduler
`, "scheduler", testutil.WithStdlib())
	if scheduler.HasError() {
		t.Fatalf("unexpected scheduler export errors: %v", testutil.ErrorMessages(scheduler.Errors))
	}

	encoded, err := io.EncodeManifest(scheduler.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	result := testutil.Check(`
local scheduler = require("scheduler")

local function handle_satisfy_yield(payload)
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
	return yield_id ~= nil or reply_to ~= nil
end

local decision = scheduler.find_next_work({
	active_yields = {
		parent = {
			yield_id = "yield",
			reply_to = "pid",
			results = {},
		},
	},
	ready = false,
	done = false,
})

if decision.type == scheduler.DECISION_TYPE.SATISFY_YIELD then
	handle_satisfy_yield(decision.payload)
end
`,
		testutil.WithStdlib(),
		testutil.WithManifest("scheduler", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected sequential scheduler payload probes to remain non-nil, got: %v", result.Errors)
	}
}

func TestLocalReplaceHarness_NamespacedImportedSchedulerDecisionPayload(t *testing.T) {
	scheduler := testutil.CheckAndExport(`
local scheduler = {}

local DECISION_TYPE = {
	EXECUTE_NODES = "execute_nodes",
	SATISFY_YIELD = "satisfy_yield",
	COMPLETE_WORKFLOW = "complete_workflow",
	NO_WORK = "no_work",
}

local function create_decision(decision_type, payload)
	return {
		type = decision_type,
		payload = payload or {},
	}
end

function scheduler.find_next_work(state)
	for parent_id, yield_info in pairs(state.active_yields) do
		return create_decision(DECISION_TYPE.SATISFY_YIELD, {
			parent_id = parent_id,
			yield_id = yield_info.yield_id,
			reply_to = yield_info.reply_to,
			results = yield_info.results or {},
		})
	end
	return create_decision(DECISION_TYPE.NO_WORK, {
		message = "No work available",
	})
end

scheduler.DECISION_TYPE = DECISION_TYPE

return scheduler
`, "scheduler", testutil.WithStdlib())
	if scheduler.HasError() {
		t.Fatalf("unexpected scheduler export errors: %v", testutil.ErrorMessages(scheduler.Errors))
	}

	encoded, err := io.EncodeManifest(scheduler.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	result := testutil.Check(`
local orchestrator = {}
orchestrator.scheduler = require("scheduler")

local function handle_satisfy_yield(payload)
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
	return yield_id ~= nil or reply_to ~= nil
end

local decision = orchestrator.scheduler.find_next_work({
	active_yields = {
		parent = {
			yield_id = "yield",
			reply_to = "pid",
			results = {},
		},
	},
})

if decision.type == orchestrator.scheduler.DECISION_TYPE.SATISFY_YIELD then
	handle_satisfy_yield(decision.payload)
end

local exit_info = {
	yield_complete = {
		parent_id = "parent",
		yield_info = {
			yield_id = "yield",
			reply_to = "pid",
			results = {},
		},
	},
}
if exit_info and exit_info.yield_complete then
	handle_satisfy_yield({
		parent_id = exit_info.yield_complete.parent_id,
		yield_id = exit_info.yield_complete.yield_info.yield_id,
		reply_to = exit_info.yield_complete.yield_info.reply_to,
		results = exit_info.yield_complete.yield_info.results,
	})
end
`,
		testutil.WithStdlib(),
		testutil.WithManifest("scheduler", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected namespaced scheduler payload to remain non-nil, got: %v", result.Errors)
	}
}

func TestLocalReplaceHarness_FullImportedSchedulerDecisionPayload(t *testing.T) {
	scheduler := testutil.CheckAndExport(`
local consts = {
	STATUS = {
		PENDING = "pending",
		RUNNING = "running",
	},
}

local scheduler = {}

local DECISION_TYPE = {
	EXECUTE_NODES = "execute_nodes",
	SATISFY_YIELD = "satisfy_yield",
	COMPLETE_WORKFLOW = "complete_workflow",
	NO_WORK = "no_work",
}

local TRIGGER_REASON = {
	YIELD_DRIVEN = "yield_driven",
	INPUT_READY = "input_ready",
	ROOT_READY = "root_ready",
}

local CONCURRENT_CONFIG = {
	MAX_CONCURRENT_NODES = 10,
	ENABLE_INPUT_CONCURRENCY = true,
	ENABLE_ROOT_CONCURRENCY = true,
}

local function create_decision(decision_type, payload)
	return {
		type = decision_type,
		payload = payload or {},
	}
end

local function create_nodes_execution(nodes)
	return create_decision(DECISION_TYPE.EXECUTE_NODES, {
		nodes = nodes,
	})
end

local function node_has_required_inputs(node_id, node_data, input_tracker)
	if not input_tracker.requirements[node_id] then
		local available = input_tracker.available[node_id] or {}
		return next(available) ~= nil
	end

	local requirements = input_tracker.requirements[node_id]
	local available = input_tracker.available[node_id] or {}

	for _, required_key in ipairs(requirements.required or {}) do
		if not available[required_key] then
			return false
		end
	end

	return true
end

local function node_has_available_inputs(node_id, input_tracker)
	local available = input_tracker.available[node_id] or {}
	return next(available) ~= nil
end

local function yield_children_complete(yield_info)
	if not yield_info.pending_children then
		return true
	end

	for child_id, status in pairs(yield_info.pending_children) do
		if status == consts.STATUS.PENDING then
			return false
		end
	end

	return true
end

local function find_yield_driven_work(state)
	for parent_id, yield_info in pairs(state.active_yields) do
		if yield_children_complete(yield_info) then
			return create_decision(DECISION_TYPE.SATISFY_YIELD, {
				parent_id = parent_id,
				yield_id = yield_info.yield_id,
				reply_to = yield_info.reply_to,
				results = yield_info.results or {},
			})
		end
	end

	local ready_yield_children = {}

	for parent_id, yield_info in pairs(state.active_yields) do
		if yield_info.pending_children then
			local has_any_pending = false
			local has_any_runnable = false
			local has_any_running = false

			for child_id, _ in pairs(yield_info.pending_children) do
				local child_node = state.nodes[child_id]
				if child_node then
					if child_node.status == consts.STATUS.RUNNING then
						has_any_running = true
					elseif child_node.status == consts.STATUS.PENDING then
						has_any_pending = true
						if node_has_required_inputs(child_id, child_node, state.input_tracker) then
							has_any_runnable = true
							table.insert(ready_yield_children, {
								node_id = child_id,
								node_type = child_node.type,
								path = yield_info.child_path or {},
								trigger_reason = TRIGGER_REASON.YIELD_DRIVEN,
								parent_id = parent_id,
							})
						end
					end
				end
			end

			if has_any_pending and not has_any_runnable and not has_any_running then
				return create_decision(DECISION_TYPE.NO_WORK, {
					message = "Yield children pending for parent " .. parent_id .. ": waiting for inputs",
				})
			end
		end
	end

	if #ready_yield_children > 0 then
		return create_nodes_execution({ ready_yield_children[1] })
	end

	return nil
end

local function find_input_ready_work(state)
	local ready_nodes = {}

	for node_id, node_data in pairs(state.nodes) do
		if node_data.status == consts.STATUS.PENDING and
			not state.active_processes[node_id] and
			state.input_tracker.requirements[node_id] and
			node_has_required_inputs(node_id, node_data, state.input_tracker) then
			local is_yield_child = false
			for _, yield_info in pairs(state.active_yields) do
				if yield_info.pending_children and yield_info.pending_children[node_id] then
					is_yield_child = true
					break
				end
			end

			if not is_yield_child then
				table.insert(ready_nodes, {
					node_id = node_id,
					node_type = node_data.type,
					path = {},
					trigger_reason = TRIGGER_REASON.INPUT_READY,
				})
			end
		end
	end

	return decide_execution_strategy(ready_nodes, CONCURRENT_CONFIG.ENABLE_INPUT_CONCURRENCY)
end

local function find_root_driven_work(state)
	local ready_nodes = {}

	for node_id, node_data in pairs(state.nodes) do
		if node_data.status == consts.STATUS.PENDING and
			not state.input_tracker.requirements[node_id] and
			node_has_available_inputs(node_id, state.input_tracker) then
			table.insert(ready_nodes, {
				node_id = node_id,
				node_type = node_data.type,
				path = {},
				trigger_reason = TRIGGER_REASON.ROOT_READY,
			})
		end
	end

	return decide_execution_strategy(ready_nodes, CONCURRENT_CONFIG.ENABLE_ROOT_CONCURRENCY)
end

function decide_execution_strategy(ready_nodes, allow_concurrent)
	if #ready_nodes == 0 then
		return nil
	elseif #ready_nodes == 1 then
		return create_nodes_execution(ready_nodes)
	elseif allow_concurrent then
		local limit = math.min(#ready_nodes, CONCURRENT_CONFIG.MAX_CONCURRENT_NODES)
		local nodes_to_execute = {}
		for i = 1, limit do
			table.insert(nodes_to_execute, ready_nodes[i])
		end
		return create_nodes_execution(nodes_to_execute)
	else
		return create_nodes_execution({ ready_nodes[1] })
	end
end

local function check_workflow_completion(state)
	if next(state.active_processes) or next(state.active_yields) then
		return nil
	end

	local has_nodes = false
	for _ in pairs(state.nodes) do
		has_nodes = true
		break
	end

	if not has_nodes then
		return create_decision(DECISION_TYPE.COMPLETE_WORKFLOW, {
			success = true,
			message = "Empty workflow completed",
		})
	end

	return nil
end

function scheduler.find_next_work(state)
	local decision = find_yield_driven_work(state)
	if decision then
		return decision
	end

	decision = find_input_ready_work(state)
	if decision then
		return decision
	end

	decision = find_root_driven_work(state)
	if decision then
		return decision
	end

	decision = check_workflow_completion(state)
	if decision then
		return decision
	end

	return create_decision(DECISION_TYPE.NO_WORK, {
		message = "No work available, waiting for events",
	})
end

scheduler.DECISION_TYPE = DECISION_TYPE

return scheduler
`, "scheduler", testutil.WithStdlib())
	if scheduler.HasError() {
		t.Fatalf("unexpected scheduler export errors: %v", testutil.ErrorMessages(scheduler.Errors))
	}

	encoded, err := io.EncodeManifest(scheduler.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest failed: %v", err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	result := testutil.Check(`
local orchestrator = {}
orchestrator.scheduler = require("scheduler")

local function handle_satisfy_yield(payload)
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
	return yield_id ~= nil or reply_to ~= nil
end

local decision = orchestrator.scheduler.find_next_work({
	nodes = {},
	active_yields = {
		parent = {
			yield_id = "yield",
			reply_to = "pid",
			results = {},
		},
	},
	active_processes = {},
	input_tracker = {
		requirements = {},
		available = {},
	},
})

if decision.type == orchestrator.scheduler.DECISION_TYPE.SATISFY_YIELD then
	handle_satisfy_yield(decision.payload)
end

local exit_info = {
	yield_complete = {
		parent_id = "parent",
		yield_info = {
			yield_id = "yield",
			reply_to = "pid",
			results = {},
		},
	},
}
if exit_info and exit_info.yield_complete then
	handle_satisfy_yield({
		parent_id = exit_info.yield_complete.parent_id,
		yield_id = exit_info.yield_complete.yield_info.yield_id,
		reply_to = exit_info.yield_complete.yield_info.reply_to,
		results = exit_info.yield_complete.yield_info.results,
	})
end
`,
		testutil.WithStdlib(),
		testutil.WithManifest("scheduler", decoded),
	)
	if result.HasError() {
		t.Fatalf("expected full scheduler payload to remain non-nil, got: %v", result.Errors)
	}
}
