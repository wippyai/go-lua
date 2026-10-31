local test = require("test")
local bridge = require("bridge")
local test_state = require("test_state")

local function inbound(overrides)
    local args = {
        provider = "discord",
        reply_component_id = "conn-1",
        reply_resource_id = "chan-1",
        channel_id = "chan-1",
        external_user_id = "ext-1",
        external_username = "external-user",
        external_display_name = "External User",
        external_message_id = "msg-1",
        text = "hello",
        trace_context = { trace_id = "trace-1" },
    }
    if type(overrides) == "table" then
        for k, v in pairs(overrides) do args[k] = v end
    end
    return args
end

local function prepare(overrides)
    return bridge._prepare_session_turn_for_test({
        inbound = inbound(overrides),
        user_id = "installer-1",
        thread_id = "thread-1",
        thread_class = "kickside.channel",
        inbound_event = "kickside.channel.events:message.inbound",
        key = "discord:conn-1:chan-1",
    })
end

local function define_tests()
    test.describe("channel bridge durable execution gate", function()
        test.before_each(function()
            test_state.reset()
            test_state.state.durable_append_dedupe = true
        end)

        test.it("prepares exactly one session turn for duplicate inbound deliveries", function()
            local first = prepare()
            local second = prepare()

            test.not_nil(first)
            test.eq("External User: hello", first.body)
            test.is_nil(second)
            test.eq(2, #test_state.state.witness)
            test.eq("append_inbound", test_state.state.witness[1].kind)
            test.eq("append_inbound", test_state.state.witness[2].kind)
        end)

        test.it("skips execution when redelivery observes an existing durable append", function()
            local first = prepare({ external_message_id = "msg-redeliver" })
            test.not_nil(first)

            local redelivered = prepare({ external_message_id = "msg-redeliver" })
            test.is_nil(redelivered)
        end)

        test.it("fails closed when the durable append errors", function()
            test_state.state.append_errors = { ["msg-append-fails"] = "append unavailable" }

            local turn, err = prepare({ external_message_id = "msg-append-fails" })

            test.is_nil(turn)
            test.not_nil(err)
            test.contains(tostring(err), "append unavailable")
        end)
    end)
end

local run_cases = test.run_cases(define_tests)
return { run = function(options) return run_cases(options) end }

