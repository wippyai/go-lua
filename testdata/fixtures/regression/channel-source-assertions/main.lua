-- Source assertions from app:bridge_turn_test, app:receive_test, and app:responder_test.
local test = require("test")
local test_state = require("test_state")
local thread_bridge = require("thread_bridge_stub")
local automations_lib = require("automations_lib_stub")

test_state.reset()
thread_bridge.append_inbound("thread-1", { external_message_id = "msg-1" }, {}, nil, nil)
thread_bridge.append_inbound("thread-1", { external_message_id = "msg-2" }, {}, nil, nil)
test.eq(2, #test_state.state.witness)
test.eq("append_inbound", test_state.state.witness[1].kind) -- bridge_turn_test:50

test_state.reset()
thread_bridge.ensure_thread({ external_message_id = "msg-1" }, {}, nil, nil)
thread_bridge.append_inbound("thread-1", { external_message_id = "msg-1" }, {}, nil, nil)
local w = test_state.state.witness
test.eq(#w, 2)
test.eq(w[1].kind, "ensure_thread") -- receive_test:171
test.eq(w[2].kind, "append_inbound") -- receive_test:188

test_state.reset()
thread_bridge.ensure_thread({ external_message_id = "msg-1" }, {}, nil, nil)
test.eq(#test_state.state.witness, 1)
test.eq(test_state.state.witness[1].kind, "ensure_thread") -- receive_test:215

test_state.reset()
local result, err = automations_lib.patch_state("auto-1", { paused = true })
test.is_nil(err)
test.eq(result.success, true)
test.eq(test_state.state.private_patches[1].component_id, "auto-1") -- responder_test:225
test.eq(test_state.state.private_patches[1].component_id, "auto-1") -- responder_test:268

test_state.reset()
result, err = automations_lib.patch_state("auto-1", { paused = false })
test.is_nil(err)
test.eq(result.success, true)
test.eq(test_state.state.private_patches[1].patch.paused, false) -- responder_test:292
