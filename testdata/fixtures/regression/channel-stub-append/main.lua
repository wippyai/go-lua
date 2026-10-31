local test_state = require("test_state")
local thread_bridge = require("thread_bridge_stub")

test_state.reset()
thread_bridge.ensure_thread({ external_message_id = "msg-1" }, {}, nil, nil)
thread_bridge.append_inbound("thread-1", { external_message_id = "msg-1" }, {}, nil, nil)

-- From bridge_turn_test:50-52, after the stub records two inbound witnesses.
local test = require("test")
test.eq(2, #test_state.state.witness)
test.eq("ensure_thread", test_state.state.witness[1].kind)
test.eq("append_inbound", test_state.state.witness[2].kind)
