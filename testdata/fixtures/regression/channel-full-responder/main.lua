local test = require("test")
local responder = require("responder")
local test_state = require("test_state")

local function define_tests()
    test.describe("channel responder controls", function()
        test.before_each(function()
            test_state.reset()
            test_state.state.public_state = {
                status = "active",
                channel = "chan-1",
                agent = "agent-1",
                trait_count = "0",
            }
        end)

        local function install_input(overrides)
            local input = {
                connection_id = "conn-1",
                agent_id = "agent-1",
                agent_name = "Bot",
                target = "channel",
                channel_id = "chan-1",
                channel_name = "general",
            }
            if type(overrides) == "table" then
                for k, v in pairs(overrides) do input[k] = v end
            end
            return input
        end

        test.it("promotes the routing keys into public_state so the lookup is indexed", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local result, err = responder.install(install_input())

            test.is_nil(err)
            test.eq(result.public_state.connection_id, "conn-1")
            test.eq(result.public_state.channel_id, "chan-1")
            test.eq(result.state.connection_id, "conn-1")
            test.eq(result.state.channel_id, "chan-1")
        end)

        test.it("declares a default system_prompt in state so the transport stops hardcoding it", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local result, err = responder.install(install_input())

            test.is_nil(err)
            test.eq(type(result.state.system_prompt), "string")
            test.eq(#result.state.system_prompt > 0, true)
        end)

        test.it("carries an explicit system_prompt through install", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local result, err = responder.install(install_input({ system_prompt = "Reply in haiku." }))

            test.is_nil(err)
            test.eq(result.state.system_prompt, "Reply in haiku.")
        end)

        test.it("normalizes lazy response policy through install", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local result, err = responder.install(install_input({
                response_policy = {
                    mode = "lazy",
                    wait_seconds = 12.8,
                    assessor_model = "class:smart",
                    prompt = "Only reply to production incidents.",
                },
            }))

            test.is_nil(err)
            test.eq(result.state.response_policy.mode, "lazy")
            test.eq(result.state.response_policy.wait_seconds, 12)
            test.eq(result.state.response_policy.assessor_model, "class:smart")
            test.eq(result.state.response_policy.prompt, "Only reply to production incidents.")
        end)

        test.it("defaults unknown response policy to immediate", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local result, err = responder.install(install_input({
                response_policy = { mode = "manual", wait_seconds = 10 },
            }))

            test.is_nil(err)
            test.eq(result.state.response_policy.mode, "immediate")
            test.is_nil(result.state.response_policy.wait_seconds)
        end)

        test.it("normalizes session rotation policy through install", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local result, err = responder.install(install_input({
                session_policy = { rotate_after_idle = true, idle_seconds = 7200.9, runtime_idle_seconds = 900.8 },
            }))

            test.is_nil(err)
            test.eq(result.state.session_policy.rotate_after_idle, true)
            test.eq(result.state.session_policy.idle_seconds, 7200)
            test.eq(result.state.session_policy.runtime_idle_seconds, 900)
        end)

        test.it("defaults and clamps live runtime idle policy", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local default_result, default_err = responder.install(install_input())
            test.is_nil(default_err)
            test.eq(default_result.state.session_policy.rotate_after_idle, true)
            test.eq(default_result.state.session_policy.idle_seconds, 7200)
            test.eq(default_result.state.session_policy.runtime_idle_seconds, 600)

            local low_result, low_err = responder.install(install_input({
                channel_id = "chan-low",
                session_policy = { runtime_idle_seconds = 12 },
            }))
            test.is_nil(low_err)
            test.eq(low_result.state.session_policy.runtime_idle_seconds, 60)

            local high_result, high_err = responder.install(install_input({
                channel_id = "chan-high",
                session_policy = { runtime_idle_seconds = 99 * 60 * 60 },
            }))
            test.is_nil(high_err)
            test.eq(high_result.state.session_policy.runtime_idle_seconds, 24 * 60 * 60)
        end)

        test.it("keeps an explicitly disabled chat rotation policy", function()
            test_state.add_component("conn-1", { provider = "discord" })
            local result, err = responder.install(install_input({
                session_policy = { rotate_after_idle = false, runtime_idle_seconds = 600 },
            }))

            test.is_nil(err)
            test.eq(result.state.session_policy.rotate_after_idle, false)
            test.is_nil(result.state.session_policy.idle_seconds)
        end)

        test.it("rejects a duplicate responder on the same connection+channel, naming the existing one", function()
            test_state.add_component("conn-1", { provider = "discord" })
            test_state.add_responder("existing-1", {
                connection_id = "conn-1",
                channel_id = "chan-1",
                title = "General Responder",
            })

            local result, err = responder.install(install_input())

            test.is_nil(result)
            test.eq(type(err) == "string" and err:find("General Responder") ~= nil, true)
        end)

        test.it("allows a responder on a different channel of the same connection", function()
            test_state.add_component("conn-1", { provider = "discord" })
            test_state.add_responder("existing-1", {
                connection_id = "conn-1",
                channel_id = "chan-other",
                title = "Other Responder",
            })

            local result, err = responder.install(install_input())
            test.is_nil(err)
            test.eq(result.public_state.channel_id, "chan-1")
        end)

        test.it("status reads the public automation state", function()
            local state, err = responder.status({ component_id = "auto-1" })

            test.is_nil(err)
            test.eq(state.status, "active")
            test.eq(test_state.state.status_reads[1], "auto-1")
        end)

        test.it("read_config returns the editable private responder state", function()
            test_state.add_responder("auto-1", {
                connection_id = "conn-1",
                channel_id = "chan-1",
                agent_id = "agent-1",
                title = "General responder",
                traits = { "cap-1" },
                context = "Only answer production incidents.",
                system_prompt = "Be concise.",
            })

            local result, err = responder.read_config({ component_id = "auto-1" })

            test.is_nil(err)
            test.eq(result.success, true)
            test.eq(result.state.agent_id, "agent-1")
            test.eq(result.state.context, "Only answer production incidents.")
            test.eq(result.public_state.status, "active")
            test.eq(test_state.state.private_reads[1], "auto-1")
        end)

        test.it("configure updates editable fields and keeps routing fixed", function()
            test_state.add_responder("auto-1", {
                connection_id = "conn-1",
                channel_id = "chan-1",
                channel_name = "general",
                agent_id = "agent-old",
                agent_name = "Old Bot",
                paused = true,
                title = "Old title",
                system_prompt = "Old prompt.",
            })

            local result, err = responder.configure({
                component_id = "auto-1",
                input = {
                    title = "New title",
                    connection_id = "other-conn",
                    channel_id = "other-channel",
                    channel_name = "other",
                    agent_id = "agent-new",
                    agent_name = "New Bot",
                    target = "channel",
                    traits = { "cap-1", "cap-1", "cap-2" },
                    trait_contexts = { ["cap-1"] = { enabled = true }, ignored = { enabled = true } },
                    context = "Answer only when named.",
                    activation_context = { enabled = true, limit = 7 },
                    response_policy = { mode = "lazy", wait_seconds = 9, assessor_model = "class:fast", prompt = "Wait for real incidents." },
                    session_policy = { rotate_after_idle = true, idle_seconds = 3600 },
                    system_prompt = "Stay brief.",
                },
            })

            test.is_nil(err)
            test.eq(result.success, true)
            test.eq(test_state.state.private_patches[1].component_id, "auto-1")
            local patch = test_state.state.private_patches[1].patch
            test.eq(patch.connection_id, "conn-1")
            test.eq(patch.channel_id, "chan-1")
            test.eq(patch.channel_name, "general")
            test.eq(patch.agent_id, "agent-new")
            test.eq(patch.agent_name, "New Bot")
            test.eq(patch.paused, true)
            test.eq(#patch.traits, 2)
            test.eq(patch.trait_contexts["cap-1"].enabled, true)
            test.is_nil(patch.trait_contexts.ignored)
            test.eq(patch.activation_context.limit, 7)
            test.eq(patch.response_policy.mode, "lazy")
            test.eq(patch.response_policy.wait_seconds, 9)
            test.eq(patch.response_policy.assessor_model, "class:fast")
            test.eq(patch.response_policy.prompt, "Wait for real incidents.")
            test.eq(patch.session_policy.rotate_after_idle, true)
            test.eq(patch.session_policy.idle_seconds, 3600)
            test.eq(test_state.state.set_meta_calls[1].component_id, "auto-1")
            test.eq(test_state.state.set_meta_calls[1].fields.title, "New title")
            test.eq(test_state.state.set_meta_calls[1].fields.status, "paused")
            test.eq(test_state.state.set_meta_calls[1].fields.channel, "general")
            test.eq(test_state.state.set_meta_calls[1].fields.agent, "New Bot")
            test.eq(test_state.state.set_meta_calls[1].fields.trait_count, "2")
        end)

        test.it("pause patches routing state and emits a status event", function()
            local real_emit = responder._emit_event
            responder._emit_event = function(component_id, etype, body)
                test_state.state.events[#test_state.state.events + 1] = {
                    component_id = component_id,
                    type = etype,
                    body = body,
                }
                return nil
            end
            local result, err = responder.pause({ component_id = "auto-1" })
            responder._emit_event = real_emit

            test.is_nil(err)
            test.eq(result.success, true)
            test.eq(result.paused, true)
            test.eq(result.status, "paused")
            test.eq(test_state.state.private_patches[1].component_id, "auto-1")
            test.eq(test_state.state.private_patches[1].patch.paused, true)
            test.eq(test_state.state.events[1].component_id, "auto-1")
            test.eq(test_state.state.events[1].type, "kickside.channel.responder.events:status.changed")
            test.eq(test_state.state.events[1].body.status, "paused")
        end)

        test.it("resume clears routing pause and emits a status event", function()
            local real_emit = responder._emit_event
            responder._emit_event = function(component_id, etype, body)
                test_state.state.events[#test_state.state.events + 1] = {
                    component_id = component_id,
                    type = etype,
                    body = body,
                }
                return nil
            end
            local result, err = responder.resume({ component_id = "auto-1" })
            responder._emit_event = real_emit

            test.is_nil(err)
            test.eq(result.success, true)
            test.eq(result.paused, false)
            test.eq(result.status, "active")
            test.eq(test_state.state.private_patches[1].patch.paused, false)
            test.eq(test_state.state.events[1].type, "kickside.channel.responder.events:status.changed")
            test.eq(test_state.state.events[1].body.status, "active")
        end)
    end)
end

local run_cases = test.run_cases(define_tests)
return { run = function(options) return run_cases(options) end }

