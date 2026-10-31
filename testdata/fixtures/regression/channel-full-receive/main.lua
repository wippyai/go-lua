local test = require("test")
local receive = require("receive")
local test_state = require("test_state")
local sql = require("sql")
local time = require("time")

local DB = "app:db"
local TABLE = "kickside_channel_sessions"
local HUB = "kickside.channel.session_hub"

-- Mirrors the kickside.channel 01_channel migration DDL for each dialect.
local function create_table()
    local db = sql.get(DB)
    if not db then error("missing test db") end
    local updated_at_default = db:type() == sql.type.POSTGRES
        and [[(to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'))]]
        or [[(strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))]]
    db:execute([[
        CREATE TABLE IF NOT EXISTS kickside_channel_sessions (
            provider    TEXT NOT NULL,
            external_id TEXT NOT NULL,
            session_id  TEXT NOT NULL,
            updated_at  TEXT NOT NULL DEFAULT ]] .. updated_at_default .. [[,
            PRIMARY KEY (provider, external_id)
        )
    ]])
    db:release()
end

local function truncate()
    local db = sql.get(DB)
    if not db then error("missing test db") end
    db:execute("DELETE FROM " .. TABLE)
    db:release()
end

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
    }
    if type(overrides) == "table" then
        for k, v in pairs(overrides) do args[k] = v end
    end
    return args
end

local function install_process_stub()
    receive._process = {
        send = function(name, topic, payload)
            table.insert(test_state.state.sent, { name = name, topic = topic, payload = payload })
            if test_state.state.send_error then return false, test_state.state.send_error end
            return true, nil
        end,
    }
end

local function seed_responder(opts)
    opts = type(opts) == "table" and opts or {}
    test_state.add_responder("auto-1", {
        connection_id = "conn-1",
        channel_id = "chan-1",
        agent_id = "agent-1",
        traits = { "trait-1" },
        trait_contexts = { ["trait-1"] = { level = "brief" } },
        context = "channel context",
        response_policy = opts.response_policy or {},
        session_policy = opts.session_policy or {},
        system_prompt = opts.system_prompt,
        paused = opts.paused == true,
    })
    -- The routed agent must exist for the turn to run.
    if opts.agent_exists ~= false then
        test_state.add_component("agent-1", { title = "Agent One" })
    end
end

local function define_tests()
    test.describe("channel receive", function()
        test.before_all(create_table)
        test.before_each(function()
            truncate()
            test_state.reset()
            install_process_stub()
        end)

        test.it("routes a channel turn via the indexed responder lookup", function()
            seed_responder()
            local result = receive.receive(inbound())

            test.eq(result.status, "routed")
            test.eq(result.session_id ~= nil, true)
            test.eq(#test_state.state.sent, 1)

            local sent = (test_state.state.sent[1] or { payload = {} }) :: any
            test.eq(sent.name, HUB)
            test.eq(sent.topic, "kickside.channel.inbound")
            test.eq(sent.payload.kind, "turn")

            local route: any = sent.payload.route or {}
            test.eq(route.key, "discord:conn-1:chan-1")
            test.eq(route.run_as.mode, "frozen")
            test.eq(route.run_as.identity.actor_id, "installer-1")
            test.eq(route.start.agent_id, "agent-1")
            test.eq(route.start.session_kind, "CHANNEL")
            test.eq(route.thread.class, "kickside.channel")
            test.eq(route.thread.inbound_event, "kickside.channel.events:message.inbound")
            test.eq(route.reply.component_id, "conn-1")
            test.eq(route.inbound.text, "hello")
            test.eq(route.inbound.trace_context.trace_id, "trace-1")
            test.eq(route.inbound.trace_context.correlation_key, "channel:discord:conn-1:chan-1:message:msg-1")
        end)

        test.it("accepts the canonical user_agent ref while checking the backing component id", function()
            test_state.add_responder("auto-1", {
                connection_id = "conn-1",
                channel_id = "chan-1",
                agent_id = "user_agent:agent-1",
            })
            test_state.add_component("agent-1", { title = "Agent One" })

            local result = receive.receive(inbound())

            test.eq(result.status, "routed")
            local route: any = (test_state.state.sent[1] or { payload = {} }).payload.route or {}
            test.eq(route.start.agent_id, "user_agent:agent-1")
        end)

        test.it("does not route a paused responder", function()
            seed_responder({ paused = true })
            local result = receive.receive(inbound())
            test.eq(result.accepted, false)
            test.eq(result.status, "unrouted")
            test.eq(#test_state.state.sent, 0)
        end)

        test.it("is unrouted when no responder covers the channel", function()
            local result = receive.receive(inbound({ channel_id = "chan-unknown", reply_resource_id = "chan-unknown" }))
            test.eq(result.accepted, false)
            test.eq(result.status, "unrouted")
            test.eq(#test_state.state.sent, 0)
        end)

        test.it("returns a clear error when the routed agent no longer exists", function()
            seed_responder({ agent_exists = false })
            local result = receive.receive(inbound())
            test.eq(result.accepted, false)
            test.eq(result.status, "error")
            test.eq(type(result.error) == "string" and result.error:find("agent") ~= nil, true)
            test.eq(#test_state.state.sent, 0)
        end)

        test.it("witnesses a paused responder's inbound on the channel thread under the installer identity", function()
            seed_responder({ paused = true })
            local result = receive.receive(inbound({ text = "still logged" }))

            -- The turn does not route (paused), but the message is witnessed.
            test.eq(result.accepted, false)
            test.eq(result.status, "unrouted")
            test.eq(#test_state.state.sent, 0)

            local w = test_state.state.witness
            test.eq(#w, 2)
            test.eq(w[1].kind, "ensure_thread")
            test.eq(w[1].actor_id, "installer-1")
            test.is_true(w[1].has_scope)
            test.eq(w[1].opts.thread_class, "kickside.channel")
            test.eq(w[2].kind, "append_inbound")
            test.eq(w[2].thread_id, "thread-witness-1")
            test.eq(w[2].opts.event_type, "kickside.channel.events:message.inbound")
            test.eq(w[2].inbound.text, "still logged")
            test.eq(w[2].actor_id, "installer-1")
        end)

        test.it("witnesses the inbound when the routed agent no longer exists", function()
            seed_responder({ agent_exists = false })
            local result = receive.receive(inbound())
            test.eq(result.status, "error")
            local w = test_state.state.witness
            test.eq(#w, 2)
            test.eq(w[2].kind, "append_inbound")
            test.eq(w[2].opts.event_type, "kickside.channel.events:message.inbound")
        end)

        test.it("skips the witness without crashing when the installer identity is unresolvable", function()
            -- The frozen installer principal was deleted since install: run_as.resolve
            -- fails. The witness is a truthful logged skip -- receive still returns the
            -- routing outcome, never crashes, and never falls back to a system identity.
            seed_responder({ paused = true })
            test_state.state.witness_run_as_err = "installer principal not found"
            local result = receive.receive(inbound({ text = "lost installer" }))
            test.eq(result.accepted, false)
            test.eq(result.status, "unrouted")
            -- No thread write happened: the witness stopped at identity resolution.
            test.eq(#test_state.state.witness, 0)
        end)

        test.it("skips the witness without crashing when the witness thread write fails", function()
            -- A persistent witness failure (ensure_thread errors) must not crash receive
            -- or change the routing result; the ingress logs and moves on (best-effort).
            seed_responder({ paused = true })
            test_state.state.witness_ensure_err = "threads upsert unavailable"
            local result = receive.receive(inbound({ text = "witness write down" }))
            test.eq(result.accepted, false)
            test.eq(result.status, "unrouted")
            -- ensure_thread was attempted (and failed); no append followed it.
            test.eq(#test_state.state.witness, 1)
            test.eq(test_state.state.witness[1].kind, "ensure_thread")
        end)

        test.it("does not witness an active routed turn at ingress -- the bridge owns that append", function()
            seed_responder()
            local result = receive.receive(inbound())
            test.eq(result.status, "routed")
            test.eq(#test_state.state.witness, 0)
        end)

        test.it("does not witness an unrouted channel with no responder -- no owner identity exists", function()
            local result = receive.receive(inbound({ channel_id = "chan-unknown", reply_resource_id = "chan-unknown" }))
            test.eq(result.status, "unrouted")
            test.eq(#test_state.state.witness, 0)
        end)

        test.it("carries the responder-configured system_prompt into the session start", function()
            seed_responder({ system_prompt = "Reply only in haiku." })
            local result = receive.receive(inbound())
            test.eq(result.status, "routed")
            local route: any = (test_state.state.sent[1] or { payload = {} }).payload.route or {}
            test.eq(route.start.system_prompt, "Reply only in haiku.")
        end)

        test.it("carries the responder response_policy into the session start", function()
            seed_responder({
                response_policy = {
                    mode = "lazy",
                    wait_seconds = 15,
                    assessor_model = "class:smart",
                    prompt = "Ignore casual channel noise.",
                },
            })
            local result = receive.receive(inbound())
            test.eq(result.status, "routed")
            local route: any = (test_state.state.sent[1] or { payload = {} }).payload.route or {}
            test.eq(route.start.response_policy.mode, "lazy")
            test.eq(route.start.response_policy.wait_seconds, 15)
            test.eq(route.start.response_policy.assessor_model, "class:smart")
            test.eq(route.start.response_policy.prompt, "Ignore casual channel noise.")
        end)

        test.it("rotates the routed chat session after configured idle age", function()
            seed_responder({
                session_policy = { rotate_after_idle = true, idle_seconds = 2 * 60 * 60, runtime_idle_seconds = 15 * 60 },
            })
            local first = receive.receive(inbound({ external_message_id = "msg-1" }))
            test.eq(first.status, "routed")
            local first_id = first.session_id

            local db = sql.get(DB)
            if not db then error("missing test db") end
            local old = time.now():utc():add(-3 * time.HOUR):format_rfc3339()
            db:execute(
                "UPDATE " .. TABLE .. " SET updated_at = $1 WHERE provider = $2 AND external_id = $3",
                { old, "discord", "chan-1" })
            db:release()

            local second = receive.receive(inbound({ external_message_id = "msg-2" }))
            test.eq(second.status, "routed")
            test.is_true(type(second.session_id) == "string" and second.session_id ~= first_id)
            local route: any = (test_state.state.sent[2] or { payload = {} }).payload.route or {}
            test.eq(route.session_id, second.session_id)
            test.eq(route.start.session_policy.rotate_after_idle, true)
            test.eq(route.start.session_policy.idle_seconds, 7200)
            test.eq(route.start.session_policy.runtime_idle_seconds, 900)
        end)

        test.it("applies the default idle rotation policy when older state has no session_policy", function()
            seed_responder()
            local first = receive.receive(inbound({ external_message_id = "msg-1" }))
            test.eq(first.status, "routed")
            local first_id = first.session_id

            local db = sql.get(DB)
            if not db then error("missing test db") end
            local old = time.now():utc():add(-3 * time.HOUR):format_rfc3339()
            db:execute(
                "UPDATE " .. TABLE .. " SET updated_at = $1 WHERE provider = $2 AND external_id = $3",
                { old, "discord", "chan-1" })
            db:release()

            local second = receive.receive(inbound({ external_message_id = "msg-2" }))
            test.eq(second.status, "routed")
            test.is_true(type(second.session_id) == "string" and second.session_id ~= first_id)
            local route: any = (test_state.state.sent[2] or { payload = {} }).payload.route or {}
            test.eq(route.start.session_policy.rotate_after_idle, true)
            test.eq(route.start.session_policy.idle_seconds, 7200)
            test.eq(route.start.session_policy.runtime_idle_seconds, 600)
        end)

        test.it("does not rotate when the responder explicitly disables idle rotation", function()
            seed_responder({
                session_policy = { rotate_after_idle = false, runtime_idle_seconds = 600 },
            })
            local first = receive.receive(inbound({ external_message_id = "msg-1" }))
            test.eq(first.status, "routed")

            local db = sql.get(DB)
            if not db then error("missing test db") end
            local old = time.now():utc():add(-3 * time.HOUR):format_rfc3339()
            db:execute(
                "UPDATE " .. TABLE .. " SET updated_at = $1 WHERE provider = $2 AND external_id = $3",
                { old, "discord", "chan-1" })
            db:release()

            local second = receive.receive(inbound({ external_message_id = "msg-2" }))
            test.eq(second.status, "routed")
            test.eq(second.session_id, first.session_id)
        end)

        test.it("does not hardcode a transport system_prompt when the responder sets none", function()
            seed_responder({ system_prompt = nil })
            local result = receive.receive(inbound())
            test.eq(result.status, "routed")
            local route: any = (test_state.state.sent[1] or { payload = {} }).payload.route or {}
            test.eq(route.start.system_prompt, "")
        end)
    end)
end

local run_cases = test.run_cases(define_tests)
return { run = function(options) return run_cases(options) end }

