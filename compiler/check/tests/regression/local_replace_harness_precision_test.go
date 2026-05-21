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
