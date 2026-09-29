-- Tests for kickside.automation.trigger:trigger_service — the kickside.trigger:service
-- surface: contract/schema registration, install-time mode dispatch + rollback
-- recording, the engine-reserved _trigger block lifecycle, and the patch_state
-- guards that keep consumers out of the key.

local test = require("test")
local contract = require("contract")
local json = require("json")
local registry = require("registry")
local automations_lib = require("automations_lib")
local resolver = require("trigger_resolver")
local trigger_service = require("trigger_service")

type Map = { [string]: any }

local CATALOG: { Map } = {
    { id = "ns:events_message", binding = "ns:events", surface = "events", event = "ns.events:message" },
    { id = "ns:pull_port", binding = "ns:pull", surface = "collection" },
}

local function with_catalog(fn: () -> ())
    local real = resolver._resolve_source
    resolver._resolve_source = function(source: string): (any?, any)
        for _, d in ipairs(CATALOG) do
            if (d :: Map).id == source then return d, nil end
        end
        return nil, "source port not found: " .. source
    end
    local ok, err = pcall(fn)
    resolver._resolve_source = real
    if not ok then error(err) end
end

-- with_mode stubs the service's mode dispatch: every mode method call lands in
-- `calls` and answers from `answers[method]`.
local function with_mode(answers: Map, fn: (calls: { Map }) -> ())
    local real_open = trigger_service._open_mode
    local calls: { Map } = {}
    trigger_service._open_mode = function(binding_id: string): (any?, string?)
        local inst: Map = {}
        for _, method in ipairs({ "attach", "detach", "set_enabled", "reconfigure", "describe" }) do
            inst[method] = function(_self: any, args: any): (any?, any?)
                calls[#calls + 1] = { binding_id = binding_id, method = method, args = args }
                return answers[method] or { success = true }, nil
            end
        end
        return inst, nil
    end
    local ok, err = pcall(function() fn(calls) end)
    trigger_service._open_mode = real_open
    if not ok then error(err) end
end

-- with_component stubs the component module under automations_lib for the
-- service's _trigger reads/writes; captures every SET_CONTEXT update.
local function with_component(private_context: Map, fn: (updates: { Map }) -> ())
    local updates: { Map } = {}
    local real_modules = automations_lib._modules
    automations_lib._modules = {
        component = {
            ACCESS = { READ = 1, WRITE = 2 },
            get_context = function(_component_id: string, _access: any): (any?, any?)
                return private_context, nil
            end,
            get_service = function(): (any?, any?)
                return {
                    update = function(_self: any, req: any): (any?, any?)
                        updates[#updates + 1] = req
                        return { success = true }, nil
                    end,
                }, nil
            end,
        },
    }
    local ok, err = pcall(function() fn(updates) end)
    automations_lib._modules = real_modules
    if not ok then error(err) end
end

-- with_racing_component stubs the component module so the first context write is
-- refused the way a compare-and-set refuses a stale writer, and the context the
-- loser re-reads is the one the winner left behind.
local function with_racing_component(first: Map, second: Map, fn: (updates: { Map }) -> ())
    local updates: { Map } = {}
    local reads = 0
    local real_modules = automations_lib._modules
    automations_lib._modules = {
        component = {
            ACCESS = { READ = 1, WRITE = 2 },
            get_context = function(_component_id: string, _access: any): (any?, any?)
                reads = reads + 1
                if reads == 1 then return first, nil end
                return second, nil
            end,
            get_service = function(): (any?, any?)
                return {
                    update = function(_self: any, req: any): (any?, any?)
                        updates[#updates + 1] = req
                        if #updates == 1 then
                            return {
                                success = false,
                                error = "private_context precondition does not hold for component auto-1",
                                error_kind = errors.CONFLICT,
                            }, nil
                        end
                        return { success = true }, nil
                    end,
                }, nil
            end,
        },
    }
    local ok, err = pcall(function() fn(updates) end)
    automations_lib._modules = real_modules
    if not ok then error(err) end
end

local function precondition_of(update: Map): Map
    local payload = ((update.commands :: { Map })[1].payload :: Map)
    return type(payload.precondition) == "table" and (payload.precondition :: Map) or {}
end

local function written_block(update: Map): Map
    local payload = ((update.commands :: { Map })[1].payload :: Map)
    local context = payload.private_context :: Map
    return type(context._trigger) == "table" and (context._trigger :: Map) or {}
end

local function collecting_recorder(): (Map, { Map })
    local steps: { Map } = {}
    return {
        rollback = function(target: string, args: Map?)
            steps[#steps + 1] = { target = target, args = args or {} }
        end,
    }, steps
end

local function installed_context(block: Map): Map
    return {
        component_id = "auto-1",
        _rollback = { { target = "ns:undo", args = {} } },
        _execution_identity = { actor_id = "owner-1", actor_context = "{}" },
        _trigger = block,
        title = "My automation",
    }
end

local function timer_block(extra: Map?): Map
    local block: Map = {
        v = 1,
        spec = { v = 1, source = "", schedule = { type = "ticker", expression = "5m" }, config = {}, filter = { expr = "" } },
        mode = "timer",
        registration = { schedule_id = "sched-1", schedule_type = "ticker" },
        phase = "live",
        state_generation = 4,
        install_id = "inst-1",
    }
    if type(extra) == "table" then
        for k, v in pairs(extra :: Map) do block[k] = v end
    end
    return block
end

local function define_tests()
    test.describe("kickside.trigger service", function()

        -- ─── contract + schema registration ─────────────────────────
        test.describe("contract entries", function()
            test.it("loads the mode and service contract definitions", function()
                local mode_def, merr = contract.get("kickside.trigger:mode")
                test.is_nil(merr)
                test.not_nil(mode_def)
                local service_def, serr = contract.get("kickside.trigger:service")
                test.is_nil(serr)
                test.not_nil(service_def)
            end)

            test.it("publishes a valid trigger spec v1 schema", function()
                local entry, err = registry.get("kickside.trigger:trigger_spec_schema")
                test.is_nil(err)
                local meta = (entry :: Map).meta :: Map
                test.eq(meta.type, "trigger.schema")
                test.eq(meta.id, "trigger.spec.v1")
                local schema, derr = json.decode(tostring(meta.json_schema))
                test.is_nil(derr)
                local s = schema :: Map
                test.eq((s.required :: { string })[1], "v")
                test.eq(((s.properties :: Map).v :: Map)["const"], 1)
                test.not_nil((s.properties :: Map).source)
                test.not_nil((s.properties :: Map).schedule)
                test.not_nil((s.properties :: Map).backfill)
            end)

            test.it("publishes a valid trigger envelope schema", function()
                local entry, err = registry.get("kickside.trigger:trigger_envelope_schema")
                test.is_nil(err)
                local meta = (entry :: Map).meta :: Map
                test.eq(meta.id, "trigger.envelope.v1")
                local schema, derr = json.decode(tostring(meta.json_schema))
                test.is_nil(derr)
                local required: { [string]: boolean } = {}
                for _, key in ipairs((schema :: Map).required :: { string }) do required[key] = true end
                test.is_true(required.item)
                test.is_true(required.occurred_at)
                test.is_true(required.trigger_id)
            end)

            test.it("registers the timer, watch, and poll mode bindings with discoverable meta", function()
                for _, want in ipairs({ "timer", "watch", "poll" }) do
                    local entry, err = registry.get("kickside.automation:trigger_mode_" .. want .. "_binding")
                    test.is_nil(err)
                    test.eq((entry :: Map).kind, "contract.binding")
                    local meta = (entry :: Map).meta :: Map
                    test.eq(meta.type, "kickside.automation.trigger_mode")
                    test.eq(meta.mode, want)
                end
            end)

            test.it("registers the canonical service binding", function()
                local entry, err = registry.get("kickside.automation:trigger_service_binding")
                test.is_nil(err)
                test.eq((entry :: Map).kind, "contract.binding")
            end)
        end)

        -- ─── install ────────────────────────────────────────────────
        test.describe("install", function()
            test.it("resolves, attaches the mode, records rollback, and returns the _trigger block", function()
                with_catalog(function()
                    with_mode({
                        attach = {
                            success = true,
                            registration = { projection_id = "proj-1", thread_id = "thread-7" },
                            rollback = {
                                { target = "kickside.automation.binding:remove_projection", args = { projection_id = "proj-1" } },
                            },
                        },
                    }, function(calls: { Map })
                        local recorder, steps = collecting_recorder()
                        local res, err = trigger_service.install({
                            spec = {
                                v = 1,
                                source = "ns:events_message",
                                config = { thread_id = "thread-7" },
                                filter = { expr = "item.text" },
                            },
                            consumer = {
                                component_id = "auto-1",
                                handler_ref = "on_event",
                                drop_event_type = "ns:dropped",
                            },
                            recorder = recorder,
                        })
                        test.is_nil(err)
                        local r = res :: Map
                        test.is_true(r.success)
                        local block = r.trigger :: Map
                        test.eq(block.v, 1)
                        test.eq(block.mode, "watch")
                        test.eq(block.phase, "live")
                        test.eq((block.registration :: Map).projection_id, "proj-1")
                        test.eq((block.spec :: Map).source, "ns:events_message")
                        test.eq(((block.spec :: Map).filter :: Map).expr, "item.text")
                        test.eq((block.spec :: Map).backfill, "none")

                        test.eq(#calls, 1)
                        test.eq(calls[1].method, "attach")
                        test.eq(((calls[1].args :: Map).consumer :: Map).component_id, "auto-1")
                        test.eq(((calls[1].args :: Map).consumer :: Map).handler_ref, "on_event")
                        test.eq((calls[1].args :: Map).enabled, true)

                        test.eq(#steps, 1)
                        test.eq(steps[1].target, "kickside.automation.binding:remove_projection")
                        test.eq((steps[1].args :: Map).projection_id, "proj-1")
                    end)
                end)
            end)

            test.it("seeds the first generation and the identity of this install", function()
                with_catalog(function()
                    local recorder = select(1, collecting_recorder())
                    with_mode({ attach = { success = true, registration = { schedule_id = "s-1" } } }, function()
                        local res, err = trigger_service.install({
                            spec = { v = 1, schedule = { type = "ticker", expression = "5m" } },
                            consumer = { component_id = "auto-1", handler_ref = "run" },
                            recorder = recorder,
                        })
                        test.is_nil(err)
                        local block = (res :: Map).trigger :: Map
                        test.eq(block.state_generation, 1)
                        test.not_nil(block.install_id)
                        test.eq(type(block.install_id), "string")
                    end)
                end)
            end)

            test.it("attaches the machine paused when enabled = false", function()
                with_catalog(function()
                    with_mode({
                        attach = {
                            success = true,
                            registration = { schedule_id = "sched-9", schedule_type = "ticker" },
                            rollback = {},
                        },
                    }, function(calls: { Map })
                        local recorder, _ = collecting_recorder()
                        local res, err = trigger_service.install({
                            spec = { v = 1, schedule = { type = "ticker", expression = "5m" } },
                            consumer = { component_id = "auto-1", handler_ref = "run" },
                            recorder = recorder,
                            enabled = false,
                        })
                        test.is_nil(err)
                        local r = res :: Map
                        test.is_true(r.success)
                        test.eq((r.trigger :: Map).phase, "paused")
                        test.eq(#calls, 1)
                        test.eq((calls[1].args :: Map).enabled, false)
                    end)
                end)
            end)

            test.it("requires the install rollback recorder", function()
                local res, _ = trigger_service.install({
                    spec = { v = 1, schedule = { type = "ticker", expression = "5m" } },
                    consumer = { component_id = "auto-1", handler_ref = "run" },
                })
                test.eq((res :: Map).success, false)
                test.eq(((res :: Map).error :: Map).code, "invalid_spec")
                test.contains(((res :: Map).error :: Map).message, "recorder")
            end)

            test.it("surfaces resolver errors structurally", function()
                with_catalog(function()
                    local recorder, _ = collecting_recorder()
                    local invalid, _ = trigger_service.install({
                        spec = { v = 1 },
                        consumer = { component_id = "auto-1", handler_ref = "run" },
                        recorder = recorder,
                    })
                    test.eq((invalid :: Map).success, false)
                    test.eq(((invalid :: Map).error :: Map).code, "invalid_spec")

                    local missing, _ = trigger_service.install({
                        spec = { v = 1, source = "ns:absent" },
                        consumer = { component_id = "auto-1", handler_ref = "run" },
                        recorder = recorder,
                    })
                    test.eq((missing :: Map).success, false)
                    test.eq(((missing :: Map).error :: Map).code, "source_not_found")

                    -- A classification whose machine has no registered binding
                    -- stays a structured mode_not_available: source resolves
                    -- (a real port), then the empty mode catalog surfaces.
                    local real_bindings = resolver._find_mode_bindings
                    resolver._find_mode_bindings = function(): (any?, any)
                        return {}, nil
                    end
                    local ok, perr = pcall(function()
                        local unavailable, _ = trigger_service.install({
                            spec = { v = 1, source = "ns:pull_port", backfill = "all" },
                            consumer = { component_id = "auto-1", handler_ref = "run" },
                            recorder = recorder,
                        })
                        test.eq((unavailable :: Map).success, false)
                        test.eq(((unavailable :: Map).error :: Map).code, "mode_not_available")
                    end)
                    resolver._find_mode_bindings = real_bindings
                    if not ok then error(perr) end
                end)
            end)
        end)

        -- ─── reconfigure ────────────────────────────────────────────
        test.describe("reconfigure", function()
            test.it("applies an in-place cadence change and stores the new spec in _trigger", function()
                with_component(installed_context(timer_block()), function(updates: { Map })
                    with_mode({
                        reconfigure = {
                            success = true,
                            registration = { schedule_id = "sched-1", schedule_type = "ticker" },
                        },
                    }, function(calls: { Map })
                        local res, err = trigger_service.reconfigure({
                            component_id = "auto-1",
                            spec = { v = 1, schedule = { type = "ticker", expression = "10m" } },
                        })
                        test.is_nil(err)
                        test.is_true((res :: Map).success)
                        test.eq(#calls, 1)
                        test.eq(calls[1].method, "reconfigure")
                        test.eq(((calls[1].args :: Map).registration :: Map).schedule_id, "sched-1")

                        test.eq(#updates, 1)
                        local next_state = ((updates[1].commands :: { Map })[1].payload :: Map).private_context :: Map
                        local block = next_state._trigger :: Map
                        test.eq(((block.spec :: Map).schedule :: Map).expression, "10m")
                        test.eq(block.mode, "timer")
                        -- Engine keys survive the trigger write verbatim.
                        test.eq(next_state.component_id, "auto-1")
                        test.eq((next_state._rollback :: { Map })[1].target, "ns:undo")
                        test.eq((next_state._execution_identity :: Map).actor_id, "owner-1")
                        test.eq(next_state.title, "My automation")
                    end)
                end)
            end)

            test.it("preserves block fields it does not know about", function()
                local block = timer_block({
                    applied_commands = { { command_id = "cmd-1", action = "skip", at = "2026-08-14T00:00:00Z" } },
                    item_failures = { ["ik-1"] = { observe_count = 3 } },
                    cursor = { offset = 12 },
                })
                with_component(installed_context(block), function(updates: { Map })
                    with_mode({
                        reconfigure = { success = true, registration = { schedule_id = "sched-1", schedule_type = "ticker" } },
                    }, function()
                        local res, err = trigger_service.reconfigure({
                            component_id = "auto-1",
                            spec = { v = 1, schedule = { type = "ticker", expression = "10m" } },
                        })
                        test.is_nil(err)
                        test.is_true((res :: Map).success)
                        local next_block = written_block(updates[1])
                        test.eq(((next_block.spec :: Map).schedule :: Map).expression, "10m")
                        test.eq((next_block.cursor :: Map).offset, 12)
                        test.eq(((next_block.applied_commands :: { Map })[1] :: Map).command_id, "cmd-1")
                        test.eq(next_block.install_id, "inst-1")
                        test.eq(next_block.state_generation, 5)
                        test.eq((precondition_of(updates[1]).expect_match :: Map)["_trigger.state_generation"], 4)
                    end)
                end)
            end)

            test.it("re-reads and reapplies its intent when another writer moved the block", function()
                local first = installed_context(timer_block())
                local second = installed_context(timer_block({ state_generation = 9 }))
                with_racing_component(first, second, function(updates: { Map })
                    with_mode({
                        reconfigure = { success = true, registration = { schedule_id = "sched-1", schedule_type = "ticker" } },
                    }, function()
                        local res, err = trigger_service.reconfigure({
                            component_id = "auto-1",
                            spec = { v = 1, schedule = { type = "ticker", expression = "10m" } },
                        })
                        test.is_nil(err)
                        test.is_true((res :: Map).success)
                        test.eq(#updates, 2)
                        test.eq((precondition_of(updates[1]).expect_match :: Map)["_trigger.state_generation"], 4)
                        test.eq((precondition_of(updates[2]).expect_match :: Map)["_trigger.state_generation"], 9)
                        test.eq(written_block(updates[2]).state_generation, 10)
                        test.eq(((written_block(updates[2]).spec :: Map).schedule :: Map).expression, "10m")
                    end)
                end)
            end)

            test.it("rejects a spec that resolves to a different machine as structural_change", function()
                with_catalog(function()
                    with_component(installed_context(timer_block()), function(_updates: { Map })
                        with_mode({}, function(calls: { Map })
                            local res, _ = trigger_service.reconfigure({
                                component_id = "auto-1",
                                spec = { v = 1, source = "ns:events_message", config = { thread_id = "t-1" } },
                            })
                            test.eq((res :: Map).success, false)
                            test.eq(((res :: Map).error :: Map).code, "structural_change")
                            test.eq(#calls, 0, "the mode is never reached on a machine switch")
                        end)
                    end)
                end)
            end)
        end)

        -- ─── pause / resume / uninstall / describe / read_state ─────
        test.describe("lifecycle", function()
            test.it("pause and resume toggle the mode and the stored phase", function()
                with_component(installed_context(timer_block()), function(updates: { Map })
                    with_mode({}, function(calls: { Map })
                        local paused, perr = trigger_service.pause({ component_id = "auto-1" })
                        test.is_nil(perr)
                        test.is_true((paused :: Map).success)
                        test.eq(calls[1].method, "set_enabled")
                        test.eq((calls[1].args :: Map).enabled, false)
                        local paused_block = ((((updates[1].commands :: { Map })[1].payload :: Map).private_context :: Map)._trigger) :: Map
                        test.eq(paused_block.phase, "paused")

                        local resumed, rerr = trigger_service.resume({ component_id = "auto-1" })
                        test.is_nil(rerr)
                        test.is_true((resumed :: Map).success)
                        test.eq(calls[2].method, "set_enabled")
                        test.eq((calls[2].args :: Map).enabled, true)
                    end)
                end)
            end)

            test.it("uninstall detaches the mode and clears the _trigger block, keeping the component", function()
                with_component(installed_context(timer_block()), function(updates: { Map })
                    with_mode({ detach = { success = true, removed = true } }, function(calls: { Map })
                        local res, err = trigger_service.uninstall({ component_id = "auto-1" })
                        test.is_nil(err)
                        test.is_true((res :: Map).success)
                        test.eq((res :: Map).removed, true)
                        test.eq(calls[1].method, "detach")

                        test.eq(#updates, 1)
                        local next_state = ((updates[1].commands :: { Map })[1].payload :: Map).private_context :: Map
                        test.is_nil(next_state._trigger)
                        test.eq(next_state.component_id, "auto-1")
                        test.eq(next_state.title, "My automation")
                    end)
                end)
            end)

            test.it("cancels every outstanding command before it clears the block", function()
                local order: { string } = {}
                local real_cancel = trigger_service._cancel_requested_commands
                local real_count = trigger_service._count_requested_commands
                trigger_service._cancel_requested_commands = function(component_id: string, reason: string): (number, any)
                    order[#order + 1] = "cancel:" .. component_id .. ":" .. reason
                    return 2, nil
                end
                trigger_service._count_requested_commands = function(_component_id: string): (number, any)
                    return 0, nil
                end
                local ok, perr = pcall(function()
                    with_component(installed_context(timer_block()), function(updates: { Map })
                        with_mode({ detach = { success = true, removed = true } }, function()
                            local res, err = trigger_service.uninstall({ component_id = "auto-1" })
                            test.is_nil(err)
                            test.is_true((res :: Map).success)
                            test.eq(order[1], "cancel:auto-1:lifecycle")
                            test.eq(#updates, 1)
                            local context = ((updates[1].commands :: { Map })[1].payload :: Map).private_context :: Map
                            test.is_nil(context._trigger)
                        end)
                    end)
                end)
                trigger_service._cancel_requested_commands = real_cancel
                trigger_service._count_requested_commands = real_count
                if not ok then error(perr) end
            end)

            test.it("refuses to clear the block while a command is still requested", function()
                local real_cancel = trigger_service._cancel_requested_commands
                local real_count = trigger_service._count_requested_commands
                trigger_service._cancel_requested_commands = function(): (number, any) return 0, nil end
                -- A command that lands between the cancel and the clear keeps the
                -- world it addressed alive until a later attempt cancels it too.
                trigger_service._count_requested_commands = function(): (number, any) return 1, nil end
                local ok, perr = pcall(function()
                    with_component(installed_context(timer_block()), function(updates: { Map })
                        with_mode({ detach = { success = true, removed = true } }, function(calls: { Map })
                            local res, err = trigger_service.uninstall({ component_id = "auto-1" })
                            test.is_nil(err)
                            test.eq((res :: Map).success, false)
                            test.eq(((res :: Map).error :: Map).code, "conflict")
                            test.eq(#calls, 0, "the machine is not detached while a command stands")
                            test.eq(#updates, 0, "the block is not cleared while a command stands")
                        end)
                    end)
                end)
                trigger_service._cancel_requested_commands = real_cancel
                trigger_service._count_requested_commands = real_count
                if not ok then error(perr) end
            end)

            test.it("gives a re-enabled trigger a world an outstanding command cannot address", function()
                local cancelled = 0
                local real_cancel = trigger_service._cancel_requested_commands
                local real_count = trigger_service._count_requested_commands
                trigger_service._cancel_requested_commands = function(): (number, any)
                    cancelled = cancelled + 1
                    return 1, nil
                end
                trigger_service._count_requested_commands = function(): (number, any) return 0, nil end
                local ok, perr = pcall(function()
                    with_catalog(function()
                        local first = timer_block()
                        with_component(installed_context(first), function()
                            with_mode({ detach = { success = true, removed = true } }, function()
                                local res = select(1, trigger_service.uninstall({ component_id = "auto-1" })) :: Map
                                test.is_true(res.success)
                                test.eq(cancelled, 1, "the command is cancelled before the world it addressed is gone")
                            end)
                        end)

                        local recorder = select(1, collecting_recorder())
                        with_mode({ attach = { success = true, registration = { schedule_id = "s-2" } } }, function()
                            local reinstalled = select(1, trigger_service.install({
                                spec = { v = 1, schedule = { type = "ticker", expression = "5m" } },
                                consumer = { component_id = "auto-1", handler_ref = "run" },
                                recorder = recorder,
                            })) :: Map
                            local block = reinstalled.trigger :: Map
                            test.eq(block.state_generation, 1, "the new world's generation restarts")
                            test.is_true(tostring(block.install_id) ~= tostring(first.install_id),
                                "a command issued against the old install can never name the new one")
                        end)
                    end)
                end)
                trigger_service._cancel_requested_commands = real_cancel
                trigger_service._count_requested_commands = real_count
                if not ok then error(perr) end
            end)

            test.it("uninstall is idempotent when no trigger is installed", function()
                with_component({ component_id = "auto-1", _rollback = {} }, function(updates: { Map })
                    with_mode({}, function(calls: { Map })
                        local res, err = trigger_service.uninstall({ component_id = "auto-1" })
                        test.is_nil(err)
                        test.is_true((res :: Map).success)
                        test.eq((res :: Map).removed, false)
                        test.eq(#calls, 0)
                        test.eq(#updates, 0)
                    end)
                end)
            end)

            test.it("describe merges mode ground truth over the stored block", function()
                with_component(installed_context(timer_block()), function(_updates: { Map })
                    with_mode({
                        describe = {
                            success = true,
                            mode = "timer",
                            phase = "paused",
                            next_run_at = "2026-07-02T10:00:00Z",
                            last_error = "",
                            raw = { schedule_id = "sched-1" },
                        },
                    }, function(_calls: { Map })
                        local res, err = trigger_service.describe({ component_id = "auto-1" })
                        test.is_nil(err)
                        local d = res :: Map
                        test.is_true(d.success)
                        test.eq(d.mode, "timer")
                        test.eq(d.phase, "paused")
                        test.eq(d.next_run_at, "2026-07-02T10:00:00Z")
                        test.eq((d.raw :: Map).schedule_id, "sched-1")
                    end)
                end)
            end)

            test.it("describe surfaces the poll cursor and drain phase from the stored block", function()
                local block: Map = {
                    v = 1,
                    spec = { v = 1, source = "ns:pull_port", config = { connection_id = "conn-1" }, schedule = { type = "ticker", expression = "30m" }, backfill = "all", filter = { expr = "" } },
                    mode = "poll",
                    registration = { poll_schedule_id = "sched-9", schedule_type = "ticker" },
                    phase = "backfilling",
                    cursor = { offset = 7 },
                }
                with_component(installed_context(block), function(_updates: { Map })
                    with_mode({
                        describe = {
                            success = true,
                            mode = "poll",
                            raw = { poll_schedule_id = "sched-9" },
                        },
                    }, function(_calls: { Map })
                        local res, err = trigger_service.describe({ component_id = "auto-1" })
                        test.is_nil(err)
                        local d = res :: Map
                        test.is_true(d.success)
                        test.eq(d.mode, "poll")
                        -- The mode reports schedule truth only; the stored drain
                        -- phase wins.
                        test.eq(d.phase, "backfilling")
                        test.eq((d.cursor :: Map).offset, 7)
                        test.eq((d.raw :: Map).poll_schedule_id, "sched-9")
                    end)
                end)
            end)

            test.it("read_state returns the stored block and not_installed when absent", function()
                with_component(installed_context(timer_block()), function(_updates: { Map })
                    local res, err = trigger_service.read_state({ component_id = "auto-1" })
                    test.is_nil(err)
                    test.eq(((res :: Map).trigger :: Map).mode, "timer")
                end)
                with_component({ component_id = "auto-1" }, function(_updates: { Map })
                    local res, _ = trigger_service.read_state({ component_id = "auto-1" })
                    test.eq((res :: Map).success, false)
                    test.eq(((res :: Map).error :: Map).code, "not_installed")
                end)
            end)
        end)

        -- ─── _trigger reserved-key ownership ────────────────────────
        test.describe("_trigger reserved key", function()
            test.it("patch_state refuses a _trigger patch before touching state", function()
                local touched = false
                local real_modules = automations_lib._modules
                automations_lib._modules = {
                    component = {
                        ACCESS = { READ = 1, WRITE = 2 },
                        get_context = function(): (any?, any?)
                            touched = true
                            return {}, nil
                        end,
                    },
                }
                local ok, perr = pcall(function()
                    local result, err = automations_lib.patch_state("auto-1", { _trigger = { mode = "timer" } })
                    test.is_nil(result)
                    test.contains(err, "_trigger is managed by the trigger service")
                    test.eq(touched, false)
                end)
                automations_lib._modules = real_modules
                if not ok then error(perr) end
            end)

            test.it("patch_state delete_keys cannot remove _trigger", function()
                with_component(installed_context(timer_block()), function(updates: { Map })
                    local result, err = automations_lib.patch_state("auto-1", { enabled = false }, {
                        delete_keys = { "_trigger", "title" },
                    })
                    test.is_nil(err)
                    test.is_true((result :: Map).success)
                    local next_state = ((updates[1].commands :: { Map })[1].payload :: Map).private_context :: Map
                    test.not_nil(next_state._trigger, "_trigger survives delete_keys")
                    test.is_nil(next_state.title, "non-reserved keys still delete")
                end)
            end)

            test.it("a declared schedule may not bind its id into _trigger via state_key", function()
                local data, err = automations_lib.install(function(_t: any)
                    return {
                        state = {},
                        schedules = {
                            {
                                method = "run",
                                schedule_type = "ticker",
                                schedule_expression = "5m",
                                state_key = "_trigger",
                            },
                        },
                    }
                end)
                test.is_nil(data)
                test.contains(tostring(err), "runtime-managed field")
            end)

            test.it("write_trigger_state is the single writer: sets and clears only the key", function()
                with_component(installed_context(timer_block()), function(updates: { Map })
                    local res, err = automations_lib.write_trigger_state("auto-1", { v = 1, mode = "watch", registration = {} })
                    test.is_nil(err)
                    test.is_true((res :: Map).success)
                    local next_state = ((updates[1].commands :: { Map })[1].payload :: Map).private_context :: Map
                    test.eq((next_state._trigger :: Map).mode, "watch")
                    test.eq(next_state.component_id, "auto-1")
                    test.eq(next_state.title, "My automation")
                    test.eq((next_state._execution_identity :: Map).actor_id, "owner-1")

                    local cleared, cerr = automations_lib.write_trigger_state("auto-1", nil)
                    test.is_nil(cerr)
                    test.is_true((cleared :: Map).success)
                    local cleared_state = ((updates[2].commands :: { Map })[1].payload :: Map).private_context :: Map
                    test.is_nil(cleared_state._trigger)
                    test.eq(cleared_state.title, "My automation")
                end)
            end)

            test.it("states the generation it was computed from and moves it forward", function()
                with_component(installed_context(timer_block()), function(updates: { Map })
                    local res, err = automations_lib.write_trigger_state("auto-1",
                        { v = 1, mode = "watch", registration = {} }, { expect_generation = 4 })
                    test.is_nil(err)
                    test.is_true((res :: Map).success)
                    test.eq((precondition_of(updates[1]).expect_match :: Map)["_trigger.state_generation"], 4)
                    test.eq(written_block(updates[1]).state_generation, 5)
                end)
            end)

            test.it("installs a first block only while the key is absent", function()
                with_component({ component_id = "auto-1", _rollback = {} }, function(updates: { Map })
                    local res, err = automations_lib.write_trigger_state("auto-1",
                        { v = 1, mode = "watch", registration = {} }, { expect_absent = true })
                    test.is_nil(err)
                    test.is_true((res :: Map).success)
                    test.eq((precondition_of(updates[1]).expect_absent :: { string })[1], "_trigger")
                    test.eq(written_block(updates[1]).state_generation, 1)
                end)
            end)

            test.it("reports a refused write as a conflict the caller can answer", function()
                local real_modules = automations_lib._modules
                automations_lib._modules = {
                    component = {
                        ACCESS = { READ = 1, WRITE = 2 },
                        get_context = function(): (any?, any?) return installed_context(timer_block()), nil end,
                        get_service = function(): (any?, any?)
                            return {
                                update = function(): (any?, any?)
                                    return { success = false, error = "precondition does not hold", error_kind = errors.CONFLICT }, nil
                                end,
                            }, nil
                        end,
                    },
                }
                local res, err, conflict = automations_lib.write_trigger_state("auto-1", { v = 1 }, { expect_generation = 4 })
                automations_lib._modules = real_modules
                test.is_nil(res)
                test.not_nil(err)
                test.is_true(conflict)
            end)

            test.it("write_trigger_state validates its inputs", function()
                local no_id, id_err = automations_lib.write_trigger_state("", {})
                test.is_nil(no_id)
                test.contains(tostring(id_err), "id is required")

                local bad_block, block_err = automations_lib.write_trigger_state("auto-1", "nope")
                test.is_nil(bad_block)
                test.contains(tostring(block_err), "table or nil")
            end)
        end)
    end)
end

local run_cases = test.run_cases(define_tests)

local function run(options: any): any
    return run_cases(options)
end

return { run = run }

