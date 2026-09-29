-- Install runner for kickside.automation types.
--
-- An install body is a closure that creates whatever the automation needs.
type Map = { [string]: any }

type BindingSpec = {
    portable_key: string,
    title: string,
    enabled: boolean,
    trigger: Map,
    flow_ref: Map,
    mapping_spec: Map,
    guard_expr: string?,
    execution_policy: Map,
    authority_scope: Map,
}

type Binding = {
    binding_id: string,
    revision: number,
    portable_key: string,
    title: string,
    enabled: boolean,
    trigger: Map,
    flow_ref: Map,
    mapping_spec: Map,
    guard_expr: string?,
    execution_policy: Map,
    authority_scope: Map,
    lowering_state: Map,
    created_by: any,
    updated_by: any,
    created_at: any,
    updated_at: any,
}

type BindingStatus = {
    enabled: boolean,
    status: string,
    schedule_type: string?,
    schedule_expression: string?,
    max_pull_pages_per_run: number?,
    max_items_per_run: number?,
    accepted_count: number?,
    last_run_accepted: number?,
}

type Transaction = {
    execute: (Transaction, string, any) -> (any?, any),
    commit: (Transaction) -> (any?, any),
    rollback: (Transaction) -> (any?, any),
}

type Database = {
    query: (Database, string, any) -> (any?, any),
    execute: (Database, string, any) -> (any?, any),
    begin: (Database) -> (Transaction?, any),
    release: (Database) -> (),
}

type Logger = {
    warn: (Logger, string, Map?) -> (),
    debug: (Logger, string, Map?) -> (),
}

type LoggerModule = {
    named: (LoggerModule, string) -> Logger,
}
-- Each step that needs cleanup registers a rollback descriptor:
--
--     t.rollback("namespace:undo_func_id", { args_table })
--
-- The descriptor is a function ID (registered registry entry, NOT an inline
-- closure) plus the args to pass at call time. The lib uses funcs.new():call
-- to invoke it, both during install-failure rollback and later when the
-- platform tears the automation down on user delete.
--
-- This keeps the rollback chain serializable: it's just an array of
-- { target = "namespace:func_id", args = {...} } records, persistable
-- alongside the state.
--
-- A successful install body returns:
--
--   {
--     state    = { ... },   -- opaque-to-platform; persisted as private_context
--     metadata = {          -- optional; persisted to kickside_component_meta
--       title       = "...",
--       icon        = "...",
--       comment     = "...",
--     },
--   }
--
-- The lib augments this with the recorded rollback chain, returning:
--
--   {
--     state    = { ... },
--     metadata = { ... },
--     rollback = { { target = "...", args = {...} }, ... },
--   }
--
-- The platform persists rollback alongside state and replays it (in reverse
-- order) when the automation is deleted.
--
-- The install body is a reusable application: it may call ANY contract (the
-- scheduler is not special — it is just one of them) using the component id the
-- engine pre-allocates and exposes via ctx, and records an undo for each effect.
-- The engine registers the component ONCE with whatever state the body returns.
--
-- Usage:
--
--   local automations = require("automations_lib")
--   local schedule = require("automation_schedule")   -- thin cron-contract helper
--   local ctx = require("ctx")
--   local function install(input)
--       return automations.install(function(t)
--           local component_id = ctx.get("component_id")  -- pre-allocated by the engine
--           t.rollback("provider.module:delete_child", { child_id = "..." })
--
--           -- recurring work: call the scheduler contract like any other, with rollback
--           local sched, err = schedule.create_action_schedule({
--               component_id = component_id, name = "run", method = "run",
--               schedule_type = "interval", schedule_expression = input.interval,
--           }, t)
--           if err then return nil, err end
--
--           return {
--               state = { child_id = "...", schedule_id = sched and sched.schedule_id },
--               metadata = { title = input.title, icon = "tabler:clock-play" },
--           }
--       end)
--   end

local automation_types = require("types")

type Dependencies = {
    types: any,
    automation_ref: any,
    execution_identity: any,
    json: any,
    contract: any,
    component: automation_types.ComponentModule,
    registry: automation_types.RegistryModule,
    funcs: automation_types.FuncsModule,
    logger: LoggerModule,
    security: any,
    autoinit: any,
    automation_schedule: any,
    trigger_resolver: any,
    view_components: any,
    sql: any,
    time: any,
    expr: any,
    ctx: any,
    uuid: any,
}

type Engine = {
    AUTOMATION_BINDING_KIND: string,
    AUTOMATION_BINDING_META_TYPE: string,
    EXECUTION_IDENTITY_KEY: string,
    SCHEDULE_TRIGGER: string,
    TRIGGER_PROGRESS_KEY: string,
    TRIGGER_STATE_KEY: string,
    _attach_binding_lowering: any,
    _build_type_composition: any,
    _capture_execution_identity: any,
    _detach_binding_lowering: any,
    _flow_runtime_binding_available: any,
    _launch_flow: any,
    _open_flow_runtime: any,
    _reset_type_composition: any,
    _revalidate_binding_dispatch: any,
    _type_action_view: any,
    call_action: any,
    class_matches: any,
    collect_actions: any,
    consumer_delivery_options: any,
    resolve_source: (string) -> (any?, any),
    component_id_for_actor: (string) -> string?,
    contract_for_method: any,
    create_binding: any,
    delete_automation: any,
    delete_binding: (string) -> (Map?, any),
    deliver_binding: any,
    disable_binding: any,
    dispatch_to_sink: any,
    enable_binding: (string) -> (Binding?, any),
    execute_binding: (any) -> (Map?, any),
    execute_binding_action: any,
    export_artifact: any,
    find_method: any,
    find_owner_by_resource: any,
    find_owners_by_resource: (string, string) -> ({ string }, any),
    get_binding: any,
    get_runtime: any,
    install: any,
    install_type: any,
    import_artifact: any,
    is_lifecycle: any,
    list_bindings: any,
    list_destinations: any,
    list_installed: any,
    list_runtime: any,
    list_sinks: any,
    list_sources: any,
    list_triggers: any,
    list_types: any,
    open_sink_writer: any,
    patch_state: any,
    update_portable_input: any,
    pause_binding: (any) -> (Map?, any),
    read_config: (string) -> (Map?, any),
    read_public_state: any,
    read_state: any,
    read_trigger_liveness: any,
    read_trigger_state: any,
    read_trigger_state_for_update: any,
    rebuild_type_composition: any,
    record_trigger_teardown: any,
    reconfigure: any,
    reconfigure_binding: any,
    replay: any,
    replace_binding: any,
    resolve_action: any,
    resolve_flow_ref: any,
    resolve_sink: any,
    resume_binding: (any) -> (Map?, any),
    uninstall: (string, any) -> (Map?, any),
    write_public_state: any,
    write_trigger_install_state: any,
    write_trigger_progress: any,
    write_trigger_state: any,
}

local function new_engine(raw_dependencies: any): Engine
local dependencies = raw_dependencies :: Dependencies
local M: Engine = {} :: Engine
local automation_ref = dependencies.automation_ref
local execution_identity = dependencies.execution_identity
local json = dependencies.json
local uuid = dependencies.uuid

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function copy_map(raw: any): Map
    local out: Map = {}
    if type(raw) ~= "table" then return out end
    for k, v in pairs(raw :: Map) do out[k] = v end
    return out
end

local function rollback_logger(): Logger
    local logger_mod = dependencies.logger :: LoggerModule
    return logger_mod:named("automations.rollback")
end

-- Reserved private_context key for the engine-owned execution identity:
-- { actor_id, actor_context }, serialized by execution_identity.to_row. Engine
-- owned so every type gets it uniformly; provider types keep only their own
-- wiring (which agent, which channel) in state.
local EXECUTION_IDENTITY_KEY = execution_identity.CONTEXT_KEY
M.EXECUTION_IDENTITY_KEY = EXECUTION_IDENTITY_KEY

-- capture_execution_identity() freezes the installing actor's identity from the
-- current security frame (install_type runs the type body under the caller's
-- authority) via the canonical execution_identity primitive and serializes it
-- to the persisted { actor_id, actor_context } carrier. The primitive drops
-- transient frame fields and fails closed without an authenticated actor +
-- named scope.
local function capture_execution_identity(): (Map?, string?)
    local id, err = execution_identity.capture("automation")
    if err or not id then return nil, err or "could not capture execution identity" end
    local row, row_err = execution_identity.to_row(id)
    if row_err or not row then return nil, row_err or "could not serialize execution identity" end
    return row :: Map, nil
end
M._capture_execution_identity = capture_execution_identity

local function pagination_int(value: any, name: string): (integer?, string?)
    if value == nil then return nil, nil end
    if type(value) ~= "number" or value ~= math.floor(value) then
        return nil, name .. " must be an integer"
    end

    local n = value :: integer
    if n < 0 then return nil, name .. " must be non-negative" end
    return n, nil
end

-- The default runner uses funcs:call to invoke the persisted target id.
-- funcs.new():call inherits the calling actor from the surrounding frame,
-- so cleanup contracts (e.g. knowledge_bases:delete) run with the caller's
-- authority automatically — no explicit with_actor needed.
--
-- Cleanup contracts return (result, err) and never raise, so the runner must
-- surface call_err to the caller. run_rollback treats a returned error as a
-- failed step the same as a raise, keeping the retain-for-retry guard live
-- when a child resource is not actually torn down.
local function default_runner(target: string, args: Map): any
    local funcs = dependencies.funcs :: automation_types.FuncsModule
    local executor, err = funcs.new()
    if err or not executor then
        error("automations.rollback: funcs.new failed: " .. tostring(err))
    end
    local _, call_err = executor:call(target, args)
    if call_err then
        local lg = rollback_logger()
        lg:warn("rollback call failed", { target = target, error = tostring(call_err) })
        return call_err
    end
    return nil
end

-- run_rollback returns the number of entries that did not clean up. Each call
-- is wrapped in pcall so a failure on entry N still attempts N-1. A step
-- counts as failed when the runner raises OR returns a non-nil error — cleanup
-- contracts signal failure by return, not by raising.
local function run_rollback(chain: { automation_types.RollbackStep }, runner: automation_types.RollbackRunner?): integer
    local fn = runner or default_runner
    local failed = 0
    -- Reverse order: last-registered cleans up first.
    for i = #chain, 1, -1 do
        local entry = chain[i]
        if entry and type(entry.target) == "string" then
            local ok, ret = pcall(fn, entry.target, entry.args or {})
            if not ok then
                failed = failed + 1
                local lg = rollback_logger()
                lg:warn("rollback runner raised", {
                    target = entry.target, error = tostring(ret),
                })
            elseif ret ~= nil then
                failed = failed + 1
                local lg = rollback_logger()
                lg:warn("rollback step returned error", {
                    target = entry.target, error = tostring(ret),
                })
            end
        end
    end
    return failed
end


-- Reserved private_context key for the trigger service's bookkeeping block
-- ({ v, spec, mode, registration, phase }). Only kickside.automation.trigger
-- writes it, through write_trigger_state below; patch_state refuses it so a
-- consumer can never mutate its own trigger registration.
local TRIGGER_STATE_KEY = "_trigger"
M.TRIGGER_STATE_KEY = TRIGGER_STATE_KEY
local TRIGGER_PROGRESS_KEY = "_trigger_progress"
local TRIGGER_INSTALL_ROLLBACK = "kickside.automation.trigger:trigger_service_install_rollback"
M.TRIGGER_PROGRESS_KEY = TRIGGER_PROGRESS_KEY
local PORTABLE_INPUT_KEY = "_portable_input"

-- State keys the engine owns on the private context. A declared schedule may not
-- bind its created id into any of these via state_key, since the engine writes
-- them itself (component_id, the public-state slot, the rollback chain, the
-- frozen identity, the accumulated schedule_ids list, and the trigger block).
local RESERVED_STATE_KEYS: { [string]: boolean } = {
    component_id = true,
    public_state = true,
    _rollback = true,
    _execution_identity = true,
    schedule_ids = true,
    [TRIGGER_STATE_KEY] = true,
    [TRIGGER_PROGRESS_KEY] = true,
    [PORTABLE_INPUT_KEY] = true,
}

-- validate_schedules checks the optional declarative `schedules` array an install
-- body returns: it must be a dense array of tables, each carrying the fields the
-- scheduler helper requires (method, schedule_type, schedule_expression), and any
-- state_key must be a non-reserved string the engine can safely write the created
-- automation schedule_id into.
local function validate_schedules(raw: any): any
    if raw == nil then return nil end
    if type(raw) ~= "table" then
        return errors.new({
            message = "automation install body `schedules` must be an array when set",
            kind = errors.INTERNAL,
        })
    end
    local max_index: integer? = nil
    local count = 0
    for k, spec in pairs(raw :: Map) do
        if type(k) ~= "number" or k < 1 or k ~= math.floor(k) then
            return errors.new({
                message = "automation install body `schedules` must be an array",
                kind = errors.INTERNAL,
            })
        end
        if type(spec) ~= "table" then
            return errors.new({
                message = "automation install body `schedules` entries must be tables",
                kind = errors.INTERNAL,
            })
        end
        local s = spec :: Map
        for _, field in ipairs({ "method", "schedule_type", "schedule_expression" }) do
            if type(s[field]) ~= "string" or s[field] == "" then
                return errors.new({
                    message = "automation install body `schedules` entry requires " .. field,
                    kind = errors.INTERNAL,
                })
            end
        end
        if s.state_key ~= nil then
            if type(s.state_key) ~= "string" or s.state_key == "" then
                return errors.new({
                    message = "automation install body `schedules` state_key must be a non-empty string",
                    kind = errors.INTERNAL,
                })
            end
            if RESERVED_STATE_KEYS[s.state_key :: string] then
                return errors.new({
                    message = "automation install body `schedules` state_key may not be a runtime-managed field: " .. tostring(s.state_key),
                    kind = errors.INTERNAL,
                })
            end
        end
        count = count + 1
        if max_index == nil or k > max_index then max_index = k end
    end
    if max_index ~= nil and max_index ~= count then
        return errors.new({
            message = "automation install body `schedules` must not contain sparse indexes",
            kind = errors.INTERNAL,
        })
    end
    return nil
end

local function validate_result(result: any): any
    if type(result) ~= "table" then
        return errors.new({
            message = "automation install body must return a table",
            kind = errors.INTERNAL,
        })
    end
    if type(result.state) ~= "table" then
        return errors.new({
            message = "automation install body must return a `state` table",
            kind = errors.INTERNAL,
        })
    end
    if result.metadata ~= nil and type(result.metadata) ~= "table" then
        return errors.new({
            message = "automation install body `metadata` must be a table when set",
            kind = errors.INTERNAL,
        })
    end
    if result.public_state ~= nil and type(result.public_state) ~= "table" then
        return errors.new({
            message = "automation install body `public_state` must be a table when set",
            kind = errors.INTERNAL,
        })
    end
    -- `created` (optional) lists the public components an install produced as
    -- { kind, id } records, so callers (e.g. the knowledge UI) can act on the
    -- new resource without re-deriving it from opaque state.
    if result.created ~= nil and type(result.created) ~= "table" then
        return errors.new({
            message = "automation install body `created` must be an array when set",
            kind = errors.INTERNAL,
        })
    end
    if type(result.created) == "table" then
        local max_index: integer? = nil
        local count = 0
        for k, v in pairs(result.created :: Map) do
            if type(k) ~= "number" or k < 1 or k ~= math.floor(k) then
                return errors.new({
                    message = "automation install body `created` must be an array",
                    kind = errors.INTERNAL,
                })
            end
            if type(v) ~= "table" then
                return errors.new({
                    message = "automation install body `created` entries must be tables",
                    kind = errors.INTERNAL,
                })
            end
            count = count + 1
            if max_index == nil or k > max_index then max_index = k end
        end
        if max_index ~= nil and max_index ~= count then
            return errors.new({
                message = "automation install body `created` must not contain sparse indexes",
                kind = errors.INTERNAL,
            })
        end
    end
    local schedules_err = validate_schedules(result.schedules)
    if schedules_err ~= nil then return schedules_err end
    return nil
end

local function normalize_rollback_chain(raw: any): ({ automation_types.RollbackStep }?, automation_types.ErrorValue?)
    if raw == nil then
        return {}, nil
    end
    if type(raw) ~= "table" then
        return nil, errors.new({
            message = "automation install result `rollback` must be an array when set",
            kind = errors.INTERNAL,
        })
    end

    local out: { automation_types.RollbackStep } = {}
    local max_index: integer? = nil
    local count = 0
    for k, step in pairs(raw :: Map) do
        if type(k) ~= "number" or k < 1 or k ~= math.floor(k) then
            return nil, errors.new({
                message = "automation install result `rollback` must be an array",
                kind = errors.INTERNAL,
            })
        end
        if type(step) ~= "table" then
            return nil, errors.new({
                message = "automation install result `rollback` entries must be tables",
                kind = errors.INTERNAL,
            })
        end
        local s: any = step
        if type(s.target) ~= "string" or s.target == "" then
            return nil, errors.new({
                message = "automation install result `rollback` entries require target",
                kind = errors.INTERNAL,
            })
        end
        if s.args ~= nil and type(s.args) ~= "table" then
            return nil, errors.new({
                message = "automation install result `rollback` entry args must be a table",
                kind = errors.INTERNAL,
            })
        end
        out[k] = {
            target = s.target,
            args = type(s.args) == "table" and (s.args :: Map) or {},
        }
        count = count + 1
        if max_index == nil or k > max_index then max_index = k end
    end
    if max_index ~= nil and max_index ~= count then
        return nil, errors.new({
            message = "automation install result `rollback` must not contain sparse indexes",
            kind = errors.INTERNAL,
        })
    end
    return out, nil
end

-- M.install runs `body` with a `t.rollback(target_id, args)` recorder. opts
-- is optional; tests can supply opts.runner to inject a recording stub
-- in place of the default funcs-based runner.
function M.install(body: any, opts: any?): (automation_types.InstallResult?, any)
    local install_opts = (type(opts) == "table" and opts or {}) :: automation_types.InstallOptions
    local runner = install_opts.runner

    local chain: { automation_types.RollbackStep } = {}
    local t: automation_types.InstallRecorder = {
        rollback = function(target_id: string, args: Map?)
            if type(target_id) == "string" and target_id ~= "" then
                chain[#chain + 1] = {
                    target = target_id,
                    args = type(args) == "table" and args or {},
                }
            end
        end,
    }

    local ok, data, err = pcall(body, t)
    if not ok then
        run_rollback(chain, runner)
        return nil, errors.wrap(data, "automation install raised")
    end
    if err ~= nil then
        run_rollback(chain, runner)
        return nil, err
    end

    local shape_err = validate_result(data)
    if shape_err ~= nil then
        run_rollback(chain, runner)
        return nil, shape_err
    end

    return ({
        state    = data.state,
        metadata = data.metadata or {},
        public_state = data.public_state,
        created  = data.created,
        schedules = data.schedules,
        rollback = chain,
    } :: automation_types.InstallResult), nil
end

-- Translate the Trigger service's opaque teardown receipt into the one
-- persisted rollback capability owned by this engine. Product install bodies
-- never receive or select a mode implementation function id.
function M.record_trigger_teardown(recorder: any, expected_component_id: any, teardown: any): string?
    if type(recorder) ~= "table" or type((recorder :: Map).rollback) ~= "function" then
        return "install recorder is required"
    end
    if type(teardown) ~= "table" then return "trigger teardown receipt is required" end
    local expected = trim(expected_component_id)
    if expected == "" then return "expected trigger component id is required" end
    local receipt = teardown :: Map
    if type(receipt.component_id) ~= "string" or receipt.component_id == ""
        or type(receipt.mode) ~= "string" or receipt.mode == ""
        or type(receipt.registration) ~= "table" then
        return "trigger teardown receipt is invalid"
    end
    if receipt.component_id ~= expected then return "trigger teardown receipt belongs to another component" end
    (recorder :: any).rollback(TRIGGER_INSTALL_ROLLBACK, receipt)
    return nil
end

-- M.replay invokes a persisted rollback chain. Same reverse-order
-- semantics as install-failure rollback. Returns (attempted_count, failed_count).
--
-- opts.options (table) is forwarded to every cleanup target as
-- args._delete_options at call time. Cleanup handlers may consult those
-- option flags (e.g. {keep_thread = true}) to skip specific steps. The
-- options bag is merged INTO each step's args, never replaces it; the
-- original persisted chain is not mutated.
function M.replay(chain: any, opts: any?): (integer, integer)
    local replay_opts = (type(opts) == "table" and opts or {}) :: automation_types.ReplayOptions
    local runner = replay_opts.runner
    if type(chain) ~= "table" then return 0, 0 end
    local source_chain = chain :: { automation_types.RollbackStep }
    local options: Map? = type(replay_opts.options) == "table" and replay_opts.options or nil
    local effective_chain: { automation_types.RollbackStep } = source_chain
    if options ~= nil then
        local cloned: { automation_types.RollbackStep } = {}
        for i, e in ipairs(source_chain) do
            if type(e) == "table" and type(e.target) == "string" then
                local merged_args: Map = {}
                if type(e.args) == "table" then
                    for k, v in pairs(e.args) do merged_args[k] = v end
                end
                merged_args._delete_options = options
                cloned[i] = { target = e.target, args = merged_args }
            else
                cloned[i] = e
            end
        end
        effective_chain = cloned
    end
    local failed = run_rollback(effective_chain, runner)
    return #source_chain, failed
end

-- ─── contract metadata helpers ─────────────────────────────────────────
--
-- The api endpoints walk binding metadata/contracts to discover what install /
-- delete / action methods are exposed. These helpers centralize the walking
-- so api/install_automation, api/list_types, api/call_action stay slim.

local LIFECYCLE_METHODS = automation_types.LIFECYCLE_METHODS

-- Pull the contracts list off a registry entry. Lua entry records expose
-- {id, kind, meta, data} — the YAML `contracts:` array lives in entry.data,
-- so entry.contracts is always nil.
local function entry_contracts(entry: automation_types.RegistryEntry?): { automation_types.BindingContract }
    if type(entry) ~= "table" then return {} end
    local data = entry.data
    if type(data) ~= "table" then return {} end
    local contracts = data.contracts
    if type(contracts) ~= "table" then return {} end
    return contracts
end

-- Find the first method target id matching `method_name` across the
-- binding's contracts. Returns (target_id, contract_id) or (nil, nil).
function M.find_method(entry: any, method_name: string): (string?, string?)
    for _, c in ipairs(entry_contracts(entry :: automation_types.RegistryEntry?)) do
        local methods = c.methods or {}
        local target = methods[method_name]
        if type(target) == "string" then return target, tostring(c.contract or "") end
    end
    return nil, nil
end

-- Return the contract id that declares the given method, or nil if no
-- contract on this binding declares it.
function M.contract_for_method(entry: any, method_name: string): string?
    local _, contract_id = M.find_method(entry, method_name)
    return contract_id
end

local function declared_action_name(spec: any): string?
    if type(spec) == "string" then return spec end
    if type(spec) == "table" and type(spec.name) == "string" then return spec.name end
    return nil
end

-- Resolve a public action declared by meta.actions. Contracts provide method
-- targets, but metadata decides which methods are callable through the generic
-- action API/UI.
function M.resolve_action(entry: any, method_name: string): automation_types.AutomationAction?
    if type(method_name) ~= "string" or method_name == "" or LIFECYCLE_METHODS[method_name] then return nil end
    if type(entry) ~= "table" then return nil end
    local meta = entry.meta
    if type(meta) ~= "table" or type(meta.actions) ~= "table" then return nil end

    for _, spec in ipairs(meta.actions :: { any }) do
        local name = declared_action_name(spec)
        if name == method_name then
            local target_id, contract_id = M.find_method(entry, method_name)
            if type(target_id) == "string" and type(contract_id) == "string" and contract_id ~= "" then
                return {
                    name = method_name,
                    contract = contract_id,
                    target = target_id,
                }
            end
            return nil
        end
    end
    return nil
end

-- Walk the binding's public action declarations. Result is a sorted array of
-- { name, contract, target } records. Undeclared contract methods stay private.
function M.collect_actions(entry: any): {automation_types.AutomationAction}
    local actions: {automation_types.AutomationAction} = {}
    local seen: { [string]: boolean } = {}
    local meta = type(entry) == "table" and entry.meta or nil
    local declarations = type(meta) == "table" and meta.actions or nil
    if type(declarations) ~= "table" then return actions end
    for _, spec in ipairs(declarations :: { any }) do
        local name = declared_action_name(spec)
        if name and not seen[name] and not LIFECYCLE_METHODS[name] then
            local action = M.resolve_action(entry, name)
            if action then
                actions[#actions + 1] = action
                seen[name] = true
            end
        end
    end
    table.sort(actions, function(a: automation_types.AutomationAction, b: automation_types.AutomationAction) return a.name < b.name end)
    return actions
end

-- True if the lifecycle table contains `method_name`.
function M.is_lifecycle(method_name: string): boolean
    return LIFECYCLE_METHODS[method_name] == true
end

-- True if the wanted class string appears in the meta.class field. Accepts
-- both string and array shapes; empty filter matches everything.
function M.class_matches(meta_class: automation_types.ClassValue?, wanted: string?): boolean
    if wanted == nil or wanted == "" then return true end
    if type(meta_class) == "string" then return meta_class == wanted end
    if type(meta_class) == "table" then
        for _, c in ipairs(meta_class :: any) do
            if c == wanted then return true end
        end
    end
    return false
end

-- ─── Platform lifecycle ops (used by HTTP api + MCP tool) ──────────────
--
-- These wrap the steps that go from "user wants to install/list/delete an
-- automation" into "umbrella component with private_context + rollback
-- chain". Both api/* and the MCP admin tool delegate to these so the
-- behaviour stays in one place.

local AUTOMATION_TYPE = automation_types.AUTOMATION_TYPE

-- All contract.binding ids carrying meta.type=kickside.automation, regardless
-- of class or announced flag. This is the discriminator for "is an umbrella
-- instance" — reading by impl id (rather than by a meta.class value) is what
-- lets list_installed see umbrellas of every class instead of only the literal
-- "automation" class. Resolved from the registry so the set has no hardcoding.
local function automation_binding_ids(): {string}
    local registry = dependencies.registry :: automation_types.RegistryModule
    local entries, err = registry.find({
        [".kind"] = "contract.binding",
        ["meta.type"] = AUTOMATION_TYPE,
    })
    if err then return {} end
    local ids: {string} = {}
    for _, e in ipairs(entries or {}) do
        if type(e.id) == "string" then ids[#ids + 1] = e.id end
    end
    -- Binding artifact shells use their own, intentionally unannounced meta.type.
    -- Include the engine-owned implementation in this projection without making
    -- it a user-visible `class=automation` row.
    ids[#ids + 1] = "kickside.automation:automation_binding_kind"
    return ids
end

local function class_array_from_meta(entry_meta: Map): {string}
    local out: {string} = {}
    if type(entry_meta.class) == "table" then
        for _, c in ipairs(entry_meta.class) do out[#out + 1] = c end
    elseif type(entry_meta.class) == "string" then
        out[#out + 1] = entry_meta.class
    end
    return out
end

local function copy_private_state(raw: any, omit_runtime: boolean?, preserve_trigger: boolean?): automation_types.AutomationPrivateContext
    local out: automation_types.AutomationPrivateContext = {}
    if type(raw) ~= "table" then return out end
    for k, v in pairs(raw :: Map) do
        local hidden_runtime = omit_runtime == true and (k == "component_id" or k == "_rollback"
            or k == EXECUTION_IDENTITY_KEY or k == PORTABLE_INPUT_KEY)
        local hidden_trigger = omit_runtime == true and preserve_trigger ~= true
            and (k == TRIGGER_STATE_KEY or k == TRIGGER_PROGRESS_KEY)
        if not hidden_runtime and not hidden_trigger then
            out[k] = v
        end
    end
    return out
end

-- Resolve automation type binding ids by a binding-meta filter (e.g.
-- { provider = "discord" }), always scoped to meta.type = automation.
-- Provider-neutral and registry-resolved, so callers (gateway hubs) never
-- hardcode an app-namespaced binding id to find their own automation types.
local function binding_ids_by_meta(type_meta: any): {string}
    local registry = dependencies.registry :: automation_types.RegistryModule
    local query: Map = { [".kind"] = "contract.binding", ["meta.type"] = AUTOMATION_TYPE }
    if type(type_meta) == "table" then
        for k, v in pairs(type_meta :: Map) do
            if type(k) == "string" and k ~= "" then query["meta." .. k] = v end
        end
    end
    local entries, err = registry.find(query)
    if err then return {} end
    local ids: {string} = {}
    for _, e in ipairs(entries or {}) do
        if type(e.id) == "string" then ids[#ids + 1] = e.id end
    end
    return ids
end

-- Reserved private_context keys the engine owns; never surfaced as provider state.
local RUNTIME_VIEW_RESERVED_STATE_KEYS: { [string]: boolean } = {
    _rollback = true,
    public_state = true,
    component_id = true,
    [EXECUTION_IDENTITY_KEY] = true,
    [TRIGGER_STATE_KEY] = true,
    [TRIGGER_PROGRESS_KEY] = true,
    [PORTABLE_INPUT_KEY] = true,
}

local function strip_reserved_state(raw: any): Map
    local out: Map = {}
    if type(raw) ~= "table" then return out end
    for k, v in pairs(raw :: Map) do
        if not RUNTIME_VIEW_RESERVED_STATE_KEYS[k] then out[k] = v end
    end
    return out
end

local function public_schema_fields(schema: any): (any?, string?)
    if schema == nil then return nil, nil end
    if type(schema) ~= "table" then return nil, "public_state_schema must be a table" end
    local fields = (schema :: Map).fields
    if type(fields) ~= "table" then return nil, "public_state_schema.fields must be a table" end
    return fields, nil
end

local function field_key(field: any): string?
    if type(field) ~= "table" then return nil end
    local key = (field :: Map).key
    if type(key) ~= "string" or key == "" then return nil end
    return key
end

local function option_value(option: any): any
    if type(option) == "table" then return (option :: Map).value end
    return option
end

local function conditional_required(field: any, public_state: any): boolean
    local rule = (field :: Map).required_if
    if type(rule) ~= "table" then return false end
    local key = (rule :: Map).key
    if type(key) ~= "string" or key == "" then return false end
    return type(public_state) == "table" and (public_state :: Map)[key] == (rule :: Map).equals
end

local function validate_public_field_value(field: any, value: any, public_state: any): string?
    local key = field_key(field) or "field"
    local kind = type((field :: Map).type) == "string" and (field :: Map).type or "text"
    local required = (field :: Map).required == true or conditional_required(field, public_state)

    if value == nil then
        if required then return key .. " is required" end
        return nil
    end

    if kind == "checkbox" or kind == "boolean" then
        if type(value) ~= "boolean" then return key .. " must be a boolean" end
        return nil
    end
    if kind == "number" then
        if type(value) ~= "number" then return key .. " must be a number" end
        return nil
    end
    if kind == "integer" then
        if type(value) ~= "number" or value ~= math.floor(value) then return key .. " must be an integer" end
        return nil
    end
    if kind == "object" then
        if type(value) ~= "table" then return key .. " must be an object" end
        return nil
    end
    if kind == "array" then
        if type(value) ~= "table" then return key .. " must be an array" end
        return nil
    end

    if type(value) ~= "string" then return key .. " must be a string" end
    if required and value == "" then return key .. " is required" end

    if kind == "select" and type((field :: Map).options) == "table" and #((field :: Map).options :: { any }) > 0 then
        for _, option in ipairs(((field :: Map).options :: { any })) do
            if value == option_value(option) then return nil end
        end
        return key .. " must be one of the declared options"
    end
    return nil
end

local function public_state_field_map(schema: any): ({ [string]: any }?, string?)
    local fields, fields_err = public_schema_fields(schema)
    if fields_err then return nil, fields_err end
    if not fields then return nil, nil end

    local allowed: { [string]: any } = {}
    for _, field in ipairs(fields :: { any }) do
        local key = field_key(field)
        if not key then return nil, "public_state_schema field is missing key" end
        allowed[key] = field
    end
    return allowed, nil
end

local function validate_public_state(schema: any, public_state: any): string?
    if public_state == nil then return nil end
    if type(public_state) ~= "table" then return "public_state must be a table" end

    local allowed, fields_err = public_state_field_map(schema)
    if fields_err then return fields_err end
    if not allowed then
        if next(public_state :: Map) ~= nil then
            return "public_state requires meta.public_state_schema"
        end
        return nil
    end

    for key, field in pairs(allowed) do
        local err = validate_public_field_value(field, (public_state :: Map)[key], public_state)
        if err then return err end
    end
    for key, _ in pairs(public_state :: Map) do
        if type(key) ~= "string" or allowed[key] == nil then
            return "public_state contains undeclared field: " .. tostring(key)
        end
    end
    return nil
end

local function coerce_public_field_read_value(field: any, value: any): (any, string?)
    if value == nil then return nil, nil end
    local key = field_key(field) or "field"
    local kind = type((field :: Map).type) == "string" and (field :: Map).type or "text"

    if kind == "checkbox" or kind == "boolean" then
        if type(value) == "boolean" then return value, nil end
        if type(value) == "string" then
            local lowered = value:lower()
            if lowered == "true" then return true, nil end
            if lowered == "false" then return false, nil end
        end
        return nil, key .. " must be a boolean"
    end

    if kind == "number" or kind == "integer" then
        local n: number? = nil
        if type(value) == "number" then
            n = value
        elseif type(value) == "string" then
            n = tonumber(value)
        end
        if n == nil then return nil, key .. " must be a " .. kind end
        if kind == "integer" and n ~= math.floor(n) then return nil, key .. " must be an integer" end
        return n, nil
    end

    if kind == "object" or kind == "array" then
        if type(value) == "table" then return value, nil end
        if type(value) == "string" then
            local decoded, derr = json.decode(value)
            if derr == nil and type(decoded) == "table" then return decoded, nil end
        end
        return nil, key .. " must be an " .. kind
    end

    return value, nil
end

local function project_public_state(schema: any, public_state: any): (Map?, any)
    if public_state == nil then return {}, nil end
    if type(public_state) ~= "table" then return nil, "public_state must be a table" end

    local allowed, fields_err = public_state_field_map(schema)
    if fields_err then return nil, fields_err end
    if not allowed then return {}, nil end

    local out: Map = {}
    for key, field in pairs(allowed) do
        local value = (public_state :: Map)[key]
        if value ~= nil then
            local coerced, cerr = coerce_public_field_read_value(field, value)
            if cerr then return nil, cerr end
            out[key] = coerced
        end
    end
    local err = validate_public_state(schema, out)
    if err then return nil, err end
    return out, nil
end

local function json_schema_type(schema: any): string?
    if type(schema) ~= "table" then return nil end
    local kind = (schema :: Map).type
    if type(kind) == "string" then return kind end
    return nil
end

local validate_input_object: (any, any, string) -> string?

-- validate_input_value enforces one declared field schema on one value: the
-- declared type, minLength for strings, the enum options, and — for an object
-- schema that declares properties — the nested object shape. A schema with no
-- declared type (a const / oneOf composite) constrains only through its enum.
local function validate_input_value(key: string, field: any, value: any): string?
    if value == nil then return nil end
    local kind = json_schema_type(field)
    if kind == "boolean" then
        if type(value) ~= "boolean" then return key .. " must be a boolean" end
    elseif kind == "number" then
        if type(value) ~= "number" then return key .. " must be a number" end
    elseif kind == "integer" then
        if type(value) ~= "number" or value ~= math.floor(value) then return key .. " must be an integer" end
    elseif kind == "object" then
        if type(value) ~= "table" then return key .. " must be an object" end
        local nested = validate_input_object(field, value, key .. ".")
        if nested then return nested end
    elseif kind == "array" then
        if type(value) ~= "table" then return key .. " must be an array" end
    elseif kind ~= nil then
        if type(value) ~= "string" then return key .. " must be a string" end
        local min_length = (field :: Map).minLength
        if type(min_length) == "number" and #value < min_length then
            return key .. " must be at least " .. tostring(min_length) .. " characters"
        end
    end

    local enum = (field :: Map).enum
    if type(enum) == "table" and #enum > 0 then
        for _, option in ipairs(enum :: { any }) do
            if value == option then return nil end
        end
        return key .. " must be one of the declared options"
    end
    return nil
end

-- validate_input_object walks one object schema level: required keys must be
-- present and non-empty, every declared property validates recursively, and a
-- key outside `properties` is rejected unless the schema opts out via
-- additionalProperties: true. A schema without `properties` is free-form.
-- `path` is the dotted prefix for error messages ("" at the top level).
validate_input_object = function(schema: any, input: any, path: string): string?
    local props = (schema :: Map).properties
    if type(props) ~= "table" then return nil end

    local required = (schema :: Map).required
    if type(required) == "table" then
        for _, key in ipairs(required :: { any }) do
            if type(key) ~= "string" or key == "" then
                return "invalid input schema: required entries must be strings"
            end
            local value = (input :: Map)[key]
            if value == nil or value == "" then return path .. key .. " is required" end
        end
    end

    for key, field in pairs(props :: Map) do
        if type(key) ~= "string" or key == "" then
            return "invalid input schema: property keys must be strings"
        end
        local err = validate_input_value(path .. key, field, (input :: Map)[key])
        if err then return err end
    end

    if (schema :: Map).additionalProperties ~= true then
        for key, _ in pairs(input :: Map) do
            if type(key) ~= "string" or (props :: Map)[key] == nil then
                return "contains undeclared field: " .. path .. tostring(key)
            end
        end
    end
    return nil
end

local function validate_install_input(schema: any, input: any): string?
    if schema == nil then return nil end
    if type(schema) ~= "table" then return "invalid input schema: inputs must be a table" end
    if input == nil then input = {} end
    if type(input) ~= "table" then return "invalid input: input must be an object" end
    local err = validate_input_object(schema, input, "")
    if err == nil then return nil end
    -- Schema-shape complaints already carry their own prefix.
    if err:sub(1, 21) == "invalid input schema:" then return err end
    return "invalid input: " .. err
end

-- Resolve an automation type's public_state_schema from its binding impl id.
-- Shared by the single-component read path (public_state_component) and the
-- list projection (list_installed), so schema resolution has one authority.
-- Returns (schema_or_nil, err). A missing schema is nil without an error: a
-- type may declare no public state.
local function public_state_schema_for_impl(registry: automation_types.RegistryModule, impl_id: string): (any?, any)
    if type(impl_id) ~= "string" or impl_id == "" then return nil, "automation not found" end
    local entry, get_err = registry.get(impl_id)
    if get_err then return nil, get_err end
    if type(entry) ~= "table" or entry.kind ~= "contract.binding" then
        return nil, "automation type not found"
    end
    local meta: Map = entry.meta or {}
    if meta.type ~= AUTOMATION_TYPE and meta.type ~= "kickside.automation.binding" then
        return nil, "binding is not an automation type"
    end
    return meta.public_state_schema, nil
end

local function public_state_component(component: automation_types.ComponentModule, registry: automation_types.RegistryModule, component_id: string, access_mask: integer?): (any?, automation_types.ComponentRow?, any)
    local rows, query_err = component.query({
        component_ids = { component_id },
        include = { access = true, meta = true },
        access_mask = access_mask or component.ACCESS.WRITE,
    })
    if query_err then return nil, nil, query_err end
    local row = rows and rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then
        return nil, nil, "automation not found"
    end
    local schema, schema_err = public_state_schema_for_impl(registry, row.impl_id)
    if schema_err then return nil, nil, schema_err end
    return schema, row :: automation_types.ComponentRow, nil
end

local function action_required_access(component: automation_types.ComponentModule, method_name: string): integer
    if method_name == "status" then return component.ACCESS.READ end
    return component.ACCESS.WRITE
end

-- Resolve a view.component registry id to its public-facing fields.
-- The frontend needs `tag_name` plus the static bundle location
-- (`base_path`/`entry_point`, or `url`) to dynamically import + mount the
-- component. Resolution delegates to the canonical wippy.views component
-- registry (which enforces meta.type == view.component); unresolved or
-- non-view-component ids return nil so callers can ignore them.
local function resolve_view_component(component_id: string?): automation_types.ViewComponent?
    if not component_id or component_id == "" then return nil end
    local view_components = dependencies.view_components
    if not view_components then return nil end
    local rec, _ = (view_components :: any).get(component_id)
    if type(rec) ~= "table" then return nil end
    local r = rec :: Map
    return {
        id       = tostring(r.id or ""),
        name     = tostring(r.name or r.id or ""),
        title    = tostring(r.title or r.name or r.id or ""),
        tag_name = tostring(r.tag_name or ""),
        base_path = tostring(r.base_path or ""),
        entry_point = tostring(r.entry_point or ""),
        url      = tostring(r.url or ""),
    } :: automation_types.ViewComponent
end

-- Enrich an automation type's `meta.component` block with resolved
-- view.component records keyed by the nested create/manage slot. Frontend
-- renders by reading `component.create_ui` etc. without needing its own
-- registry lookup or hard-coded id→url map.
local function enrich_component_block(meta_component: any): Map?
    if type(meta_component) ~= "table" then return nil end
    local out: Map = {}
    for k, v in pairs(meta_component :: Map) do out[k] = v end
    local create_ui_id: string? = nil
    local manage_ui_id: string? = nil
    local raw_create = (meta_component :: any).create
    local raw_manage = (meta_component :: any).manage
    local create_view = type(raw_create) == "table" and (raw_create :: any).view or nil
    local manage_view = type(raw_manage) == "table" and (raw_manage :: any).view or nil
    if type(create_view) == "string" then
        create_ui_id = create_view
    end
    if type(manage_view) == "string" then
        manage_ui_id = manage_view
    end
    local create_ui = resolve_view_component(create_ui_id)
    local manage_ui = resolve_view_component(manage_ui_id)
    if create_ui then out.create_ui = create_ui end
    if manage_ui then out.manage_ui = manage_ui end
    return out
end

local function metadata_string(value: any, fallback: string): string
    if type(value) == "string" then return value end
    return fallback
end

local function safe_component_metadata(metadata: Map, fallback_title: string?): any
    local out: Map = {
        title = metadata_string(metadata.title, fallback_title or "Untitled"),
        icon = metadata_string(metadata.icon, ""),
        comment = metadata_string(metadata.comment, ""),
    }
    return out
end

-- list_types: enumerate installable automation types from the registry.
-- opts: { class? = "automation"|"knowledge"|..., category? = string }
function M.list_types(opts: automation_types.ListTypesOptions?): ({automation_types.AutomationType}?, any)
    opts = opts or {}
    local registry = dependencies.registry :: automation_types.RegistryModule

    -- Provider-owned catalog adapters may be registry.entry declarations with
    -- a component create view, while executable automation implementations are
    -- contract bindings. Query the shared metadata discriminator here and let
    -- the catalog-entry guard below admit only those two supported shapes. The
    -- engine's own generic binding kind carries its own discriminator, so it is
    -- read separately and joined into the one catalog every surface reads.
    local entries, err = registry.find({ ["meta.type"] = AUTOMATION_TYPE })
    if err then return nil, err end
    local binding_entries, binding_err = registry.find({
        [".kind"] = "contract.binding",
        ["meta.type"] = automation_types.AUTOMATION_BINDING_TYPE,
    })
    if binding_err then return nil, binding_err end
    local all_entries: { any } = {}
    for _, entry in ipairs(entries or {}) do all_entries[#all_entries + 1] = entry end
    for _, entry in ipairs(binding_entries or {}) do all_entries[#all_entries + 1] = entry end

    local class_filter = opts.class
    local category_filter = opts.category

    local types: {automation_types.AutomationType} = {}
    for _, entry in ipairs(all_entries) do
        local typed_entry = entry :: automation_types.RegistryEntry
        local meta: Map = typed_entry.meta or {}
        local kind = tostring((typed_entry :: any).kind or "")
        local create = type(meta.component) == "table" and (meta.component :: any).create or nil
        local catalog_entry = kind == "contract.binding"
            or (kind == "registry.entry" and type(create) == "table" and type((create :: any).view) == "string")
        if catalog_entry
            and M.class_matches(meta.class, class_filter)
            and (category_filter == nil or category_filter == "" or meta.category == category_filter)
        then
            types[#types + 1] = {
                id             = typed_entry.id,
                name           = typed_entry.id,
                title          = meta.title or typed_entry.id,
                description    = meta.comment or "",
                icon           = meta.icon or "",
                category       = meta.category or "",
                class          = meta.class :: automation_types.ClassValue?,
                primary        = meta.primary == true,
                -- An unannounced kind is a real type with real rows; it is simply
                -- not offered in the install catalog.
                installable    = meta.announced ~= false,
                reconfigurable = meta.reconfigurable == true,
                exportable     = meta.exportable == true,
                inputs         = meta.inputs,
                component      = enrich_component_block(meta.component),
                source         = meta.source,
                public_state_schema = meta.public_state_schema,
                delete_options = meta.delete_options,
                actions        = M.collect_actions(typed_entry),
            }
        end
    end
    table.sort(types, function(a: automation_types.AutomationType, b: automation_types.AutomationType) return (a.title or "") < (b.title or "") end)
    return types, nil
end


-- ─── the port catalog (first-class kickside.automation.port entries) ─────────
-- A published connection point on a binding is a registry.entry with
-- meta.type = kickside.automation.port. It carries `binding` (the backing) and
-- exactly one kind marker: `event` (an events port, a real reference to a
-- kickside.core.threads.event entry), `operations` (a store port, the writable
-- ABI capability map), or neither (a collection port, whose backing implements
-- kickside.data:pullable). Publication is existence: an internal/audit event has
-- no port. The catalog walks the port entries and DERIVES each descriptor once --
-- kind, output face, class, store tier, presentation -- and every downstream
-- consumer (resolver, sync, UI) reads the derived descriptor, never re-derives.
--
-- Descriptor identity: `id` is the PORT ENTRY id; `binding` is the owning binding
-- id for runtime open + presentation. Stored trigger specs reference port entry
-- ids. The descriptor speaks the canonical vocabulary: `surface` is the kind,
-- `event` the events reference, `config_schema` the
-- config face, `output_schema`/`input_schema` the data faces, `operations` the
-- store ABI.
local PORT_META_TYPE = "kickside.automation.port"
local TRIGGER_META_TYPE = "kickside.automation.trigger"
local SCHEDULE_TRIGGER = "kickside.automation:periodic_trigger"
local EVENT_META_TYPE = "kickside.core.threads.event"
local PULLABLE_CONTRACT = "kickside.data:pullable"
local FLOW_RUNTIME_CONTRACT = "kickside.automation:flow_runtime"
local TRIGGER_DROPPED_EVENT = "kickside.automation.events:trigger.dropped"

-- Read a port entry's declaration field. Live registry entries carry non-standard
-- top-level yaml keys under `.data`; test fixtures may set them directly.
local function entry_field(entry: any, key: string): any
    if type(entry) == "table" then
        local data = (entry :: any).data
        if type(data) == "table" and (data :: Map)[key] ~= nil then
            return (data :: Map)[key]
        end
        return (entry :: Map)[key]
    end
    return nil
end

-- The first declared class of a binding (meta.class is a string or a string list).
local function first_class(meta: Map): string
    local cls = meta.class
    if type(cls) == "string" then return cls :: string end
    if type(cls) == "table" and type((cls :: { any })[1]) == "string" then return tostring((cls :: { any })[1]) end
    return ""
end

-- Whether a binding entry implements a data contract (non-empty method on it).
local function binding_implements(binding_entry: any, contract_id: string): boolean
    local data = type(binding_entry) == "table" and (binding_entry :: any).data or nil
    local contracts = type(data) == "table" and (data :: Map).contracts or nil
    if type(contracts) ~= "table" then return false end
    for _, raw in ipairs(contracts :: { any }) do
        local c = type(raw) == "table" and (raw :: Map) or {}
        if c.contract == contract_id then return true end
    end
    return false
end

-- Whether a store port's operations map declares an operation. The canonical form
-- is a map ({ upsert = {...} }); validation rejects every other encoding.
local function declares_operation(operations: any, name: string): boolean
    if type(operations) ~= "table" then return false end
    return (operations :: Map)[name] ~= nil
end

-- The write-capability tier of a store port: "full" when it declares the whole
-- upsert + delete + list_keys set (written items can be reconciled and retracted),
-- "append_only" when it only receives writes.
local function store_tier(operations: any): string
    if declares_operation(operations, "upsert")
        and declares_operation(operations, "delete")
        and declares_operation(operations, "list_keys") then
        return "full"
    end
    return "append_only"
end

-- A registry declaration carries a JSON schema either as a yaml mapping or as
-- the JSON text of a block scalar. Both forms decode to the same object here, so
-- every consumer of a declared schema reads one shape.
local function decode_json_table(raw: any): Map
    if type(raw) == "table" then return raw :: Map end
    if type(raw) ~= "string" or raw == "" then return {} end
    local decoded, err = json.decode(raw)
    if err or type(decoded) ~= "table" then return {} end
    return decoded :: Map
end

local function schema_or_nil(raw: any): any?
    local schema = decode_json_table(raw)
    if next(schema) == nil then return nil end
    return schema
end

-- Resolve the output face of an events port from its referenced event-type entry's
-- schema (the event's one home). Returns nil when the event entry cannot be read.
local function event_output_face(registry: automation_types.RegistryModule, event_id: string): any
    if type(event_id) ~= "string" or event_id == "" then return nil end
    local entry, err = registry.get(event_id)
    if err or type(entry) ~= "table" then return nil end
    local schema = (entry :: any).schema
    if schema ~= nil then return schema_or_nil(schema) end
    local data = (entry :: any).data
    if type(data) == "table" then return schema_or_nil((data :: Map).schema) end
    return nil
end

-- port_descriptor derives one catalog descriptor from a port entry and its owning
-- binding, by the ordered precedence markers > pullable backing. Returns nil + an
-- error string when the port cannot be classified or its backing is missing.
local function port_descriptor(registry: automation_types.RegistryModule, port_entry: any, binding_index: { [string]: any }): (Map?, string?)
    local typed_port = port_entry :: automation_types.RegistryEntry
    local port_meta: Map = typed_port.meta or {}
    local binding = tostring(entry_field(port_entry, "binding") or "")
    if binding == "" then return nil, tostring(typed_port.id) .. " declares no binding" end
    local binding_entry = binding_index[binding]
    if binding_entry == nil then return nil, tostring(typed_port.id) .. " binding not found: " .. binding end
    local binding_meta: Map = (type((binding_entry :: any).meta) == "table" and (binding_entry :: any).meta or {}) :: Map

    local event = entry_field(port_entry, "event")
    local operations = entry_field(port_entry, "operations")
    -- A port declares its data faces as JSON schema text; the catalog serves them
    -- decoded, so an install input built from a descriptor face (a data sync
    -- carries its source face as output_schema) matches the object shape the
    -- installing type declares in meta.inputs.
    local output_schema = schema_or_nil(entry_field(port_entry, "output_schema"))
    local input_schema = schema_or_nil(entry_field(port_entry, "input_schema"))
    local input_mode = entry_field(port_entry, "input_mode")

    -- A component-picker selector picks an instance of the port's own binding,
    -- so the binding's thread_class is the picker's listing filter. Carry it onto
    -- the field (the schema form forwards a field's `class` to the picker) on a
    -- copy, since the registry entry is shared. A binding that declares no
    -- thread_class yields an unfiltered picker: that is a missing declaration on
    -- the binding, not something to infer here.
    local config_schema = entry_field(port_entry, "config_schema")
    local binding_thread_class = tostring(binding_meta.thread_class or "")
    if type(config_schema) == "table" and binding_thread_class ~= "" then
        local enriched: Map = {}
        for field_key, field_value in pairs(config_schema :: Map) do
            if type(field_value) == "table"
                and tostring((field_value :: Map).picker or "") == "wc-component-picker"
                and (field_value :: Map).class == nil then
                local field_copy: Map = {}
                for k, v in pairs(field_value :: Map) do field_copy[k] = v end
                field_copy.class = binding_thread_class
                enriched[field_key] = field_copy
            else
                enriched[field_key] = field_value
            end
        end
        config_schema = enriched
    end
    local desc: Map = {
        id = typed_port.id,
        binding = binding,
        class = first_class(binding_meta),
        config_schema = config_schema,
        title = (type(port_meta.title) == "string" and port_meta.title ~= "" and port_meta.title) or typed_port.id,
        name = (type(port_meta.title) == "string" and port_meta.title ~= "" and port_meta.title) or typed_port.id,
        binding_title = (type(binding_meta.title) == "string" and binding_meta.title ~= "" and binding_meta.title) or binding,
        description = port_meta.comment or binding_meta.comment or "",
        icon = port_meta.icon or binding_meta.icon or "",
        provider = binding_meta.provider or "",
        group = binding_meta.group or "",
    }
    local component_block = enrich_component_block(binding_meta.component)
    if component_block then desc.component = component_block end

    if type(event) == "string" and event ~= "" then
        -- events port: output face derived from the referenced event-type entry's
        -- schema (the event's one home).
        desc.surface = "events"
        desc.event = event
        desc.output_schema = event_output_face(registry, event :: string)
    elseif operations ~= nil then
        desc.surface = "store"
        desc.operations = operations
        desc.input_schema = input_schema
        desc.output_schema = output_schema
        if type(input_mode) == "string" and input_mode ~= "" then desc.input_mode = input_mode end
        desc.store_tier = store_tier(operations)
    elseif binding_implements(binding_entry, PULLABLE_CONTRACT) then
        desc.surface = "collection"
        desc.output_schema = output_schema
        local reconcile = entry_field(port_entry, "reconcile")
        if reconcile ~= nil then desc.reconcile = reconcile end
    else
        return nil, tostring(typed_port.id) .. " declares no kind marker and its binding is not pullable"
    end
    return desc, nil
end

-- build_catalog derives a descriptor for every port entry, indexed against the
-- contract.binding entries that back them. A port that fails to classify is logged
-- and skipped so one bad declaration never blanks the catalog.
-- The registry version token gating the catalog snapshot: an opaque comparable
-- value naming the registry view this runtime currently serves. nil on a
-- runtime without the capability — the caller then builds per call, exactly
-- the pre-snapshot behavior, so freshness is never emulated with a clock.
local function registry_version_token(): any
    local registry = dependencies.registry
    if type((registry :: any).current_version) ~= "function" then return nil end
    local version, verr = (registry :: any).current_version()
    if verr ~= nil or version == nil then return nil end
    return (version :: any):id()
end

-- One immutable catalog per registry version. Port resolution runs on every
-- delivered event batch and every poll tick; rebuilding the catalog there
-- means materializing every contract.binding per event, which is what burned
-- prod flat. The double-token read publishes a snapshot only when the registry
-- view held still across the build; a racing change discards the build and the
-- next caller rebuilds against the newer view. A version change replaces the
-- whole snapshot, so a renamed or deleted port fails closed instead of
-- resolving from a stale catalog.
local catalog_snapshot: Map = {}

local function build_catalog(): ({Map}?, any)
    local token = registry_version_token()
    if token ~= nil and catalog_snapshot.token == token and catalog_snapshot.list ~= nil then
        return catalog_snapshot.list :: {Map}, nil
    end

    local registry = dependencies.registry :: automation_types.RegistryModule
    local ports, perr = registry.find({ [".kind"] = "registry.entry", ["meta.type"] = PORT_META_TYPE })
    if perr then return nil, perr end
    local bindings, berr = registry.find({ [".kind"] = "contract.binding" })
    if berr then return nil, berr end

    local binding_index: { [string]: any } = {}
    for _, b in ipairs(bindings or {}) do
        binding_index[tostring((b :: automation_types.RegistryEntry).id or "")] = b
    end

    local out: {Map} = {}
    local by_id: { [string]: Map } = {}
    for _, port_entry in ipairs(ports or {}) do
        local desc, derr = port_descriptor(registry, port_entry, binding_index)
        if desc then
            out[#out + 1] = desc
            by_id[tostring((desc :: Map).id or "")] = desc
        elseif derr then
            local logger_mod = dependencies.logger
            if logger_mod then
                (logger_mod :: any):named("automations.catalog"):error("port entry skipped", { error = derr })
            end
        end
    end
    table.sort(out, function(a: Map, b: Map) return tostring(a.title or "") < tostring(b.title or "") end)

    if token ~= nil and registry_version_token() == token then
        catalog_snapshot.token = token
        catalog_snapshot.list = out
        catalog_snapshot.by_id = by_id
    end
    return out, nil
end

-- Snapshot seam for tests: forces the next build to run afresh.
function M._reset_catalog_snapshot(): ()
    catalog_snapshot.token = nil
    catalog_snapshot.list = nil
    catalog_snapshot.by_id = nil
end

-- Partition the catalog by data direction, derived from the port surface: store
-- ports flow IN (sinks); collection and events ports flow OUT (sources).
local function surface_is_dir(surface: string, dir: string): boolean
    if dir == "in" then return surface == "store" end
    return surface == "collection" or surface == "events"
end

local function list_io(dir: string): ({Map}?, any)
    local catalog, err = build_catalog()
    if err then return nil, err end
    local out: {Map} = {}
    for _, desc in ipairs(catalog or {}) do
        if surface_is_dir(tostring((desc :: Map).surface or ""), dir) then out[#out + 1] = desc end
    end
    return out, nil
end

-- list_sinks: every store port (surface="store"). The binding's
-- kickside.data:writable.write is the backing. Every row carries store_tier
-- ("full" | "append_only") — consumers read the computed field, not operations.
function M.list_sinks(): ({Map}?, any)
    local list, err = list_io("in")
    if err then return nil, err end
    return (list :: {Map}?), nil
end

-- list_sources: every source port (surface="collection" | "events"). Collection
-- sources are pulled through their kickside.data:pullable backing; events sources
-- are component thread events consumed by an automation-owned projection. Consumers
-- read the computed surface, not dir/mode.
function M.list_sources(): ({Map}?, any)
    local list, err = list_io("out")
    if err then return nil, err end
    return (list :: {Map}?), nil
end

local function entry_meta_table(entry: any): Map
    if type(entry) ~= "table" then return {} end
    local meta = (entry :: any).meta
    if type(meta) == "table" then return meta :: Map end
    local data = (entry :: any).data
    if type(data) == "table" and type((data :: Map).meta) == "table" then
        return (data :: Map).meta :: Map
    end
    return {}
end

local function descriptor_id(entry: any): string
    if type(entry) ~= "table" then return "" end
    return tostring((entry :: Map).id or "")
end

local function explicit_trigger(entry: any): Map
    local meta = entry_meta_table(entry)
    return {
        id = descriptor_id(entry),
        kind = meta.kind,
        title = meta.title or descriptor_id(entry),
        portable_key = meta.portable_key,
        context_schema = meta.context_schema,
        selector_schema = meta.selector_schema,
        source = meta.source,
        bind = meta.bind,
        invoke = meta.invoke,
        ui = meta.ui,
    }
end

local function trigger_from_source(desc: Map): Map?
    local surface = tostring(desc.surface or "")
    local kind = surface == "events" and "event" or (surface == "collection" and "collection_poll" or "")
    if kind == "" then return nil end
    local source: Map = {
        port = desc.id,
        binding = desc.binding,
        surface = desc.surface,
        class = desc.class,
        component = desc.component,
        -- The port's provider, so a lowered trigger names the connection family it
        -- polls without re-reading its binding.
        provider = desc.provider,
    }
    if desc.event ~= nil then source.event = desc.event end
    return {
        id = desc.id,
        kind = kind,
        title = desc.title,
        portable_key = desc.id,
        context_schema = desc.output_schema,
        selector_schema = desc.config_schema,
        source = source,
        bind = { required_access = "write", scope = "component" },
        ui = {
            icon = desc.icon,
            group = desc.group,
            description = desc.description,
        },
    }
end

-- list_triggers: explicit trigger declarations plus declarations derived from
-- source ports. Source ports remain the runtime lowering; this catalog is inert.
function M.list_triggers(opts: Map?): ({Map}?, any)
    local registry = dependencies.registry :: automation_types.RegistryModule
    opts = type(opts) == "table" and opts or {}
    local component_impl = ""
    local component_id = trim((opts :: Map).component_id)
    if component_id ~= "" then
        local component = dependencies.component :: automation_types.ComponentModule
        local rows, component_err = component.query({
            component_ids = { component_id },
            include = { meta = false, access = true },
            access_mask = component.ACCESS.READ,
        })
        if component_err then return nil, component_err end
        if not rows or not rows[1] then return nil, "component not found: " .. component_id end
        component_impl = trim((rows[1] :: automation_types.ComponentRow).impl_id)
        if component_impl == "" then return nil, "component implementation unavailable" end
    end
    local out: { Map } = {}

    local entries, err = registry.find({ [".kind"] = "registry.entry", ["meta.type"] = TRIGGER_META_TYPE })
    if err then return nil, err end
    for _, entry in ipairs(entries or {}) do
        local point = explicit_trigger(entry)
        local source = type(point.source) == "table" and (point.source :: Map) or {}
        if component_impl == "" or point.kind == "schedule" or trim(source.binding) == component_impl then
            out[#out + 1] = point
        end
    end

    local sources, serr = M.list_sources()
    if serr then return nil, serr end
    for _, desc in ipairs(sources or {}) do
        local d = desc :: Map
        local point = (component_impl == "" or trim(d.binding) == component_impl) and trigger_from_source(d) or nil
        if point then out[#out + 1] = point end
    end

    table.sort(out, function(a: Map, b: Map) return tostring(a.title or "") < tostring(b.title or "") end)
    return out, nil
end

local function exact_flow_ref(raw: any): (Map?, string?)
    if type(raw) ~= "table" then return nil, "flow_ref is required" end
    local ref = raw :: Map
    if trim(ref.kind) ~= "flow" then return nil, "flow_ref.kind must be flow" end
    local id = trim(ref.id)
    if id == "" then return nil, "flow_ref.id is required" end
    local version = ref.version
    if type(version) ~= "number" or version < 1 or version ~= math.floor(version) then
        return nil, "flow_ref.version must be a positive integer"
    end
    return { kind = "flow", id = id, version = version }, nil
end

M._open_flow_runtime = function(authority: any?): (any?, string?)
    local contract_mod = dependencies.contract
    local definition, definition_err = (contract_mod :: any).get(FLOW_RUNTIME_CONTRACT)
    if definition_err or not definition then
        return nil, "Flow runtime unavailable: " .. tostring(definition_err or "not found")
    end
    local opener = definition
    if type(authority) == "table" then
        local actor = (authority :: Map).actor
        local scope = (authority :: Map).scope
        if actor == nil or scope == nil then return nil, "Flow runtime authority requires actor and scope" end
        opener = opener:with_actor(actor):with_scope(scope)
    end
    local runtime, open_err = opener:open()
    if open_err or not runtime then return nil, "Flow runtime open failed: " .. tostring(open_err or "not found") end
    return runtime, nil
end

-- Flow providers are optional. Discovery distinguishes "no provider installed"
-- from a provider that is present but cannot be opened: the former is an empty
-- catalog, while the latter is a real integration failure. Reading the declared
-- default binding keeps this decision structural instead of interpreting runtime
-- error text.
M._flow_runtime_binding_available = function(): (boolean?, any)
    local registry = dependencies.registry :: automation_types.RegistryModule
    local bindings, err = registry.find({ [".kind"] = "contract.binding" })
    if err then return nil, err end
    for _, entry in ipairs(bindings or {}) do
        local data = type(entry) == "table" and type((entry :: any).data) == "table"
            and ((entry :: any).data :: Map)
            or (type(entry) == "table" and (entry :: Map) or {})
        for _, raw in ipairs(type(data.contracts) == "table" and (data.contracts :: { any }) or {}) do
            local declaration = type(raw) == "table" and (raw :: Map) or {}
            if trim(declaration.contract) == FLOW_RUNTIME_CONTRACT and declaration.default == true then
                return true, nil
            end
        end
    end
    return false, nil
end

function M.resolve_flow_ref(raw_ref: any, purpose: string?): (Map?, any)
    local ref, ref_err = exact_flow_ref(raw_ref)
    if ref_err or not ref then return nil, ref_err end
    local runtime, runtime_err = M._open_flow_runtime()
    if runtime_err or not runtime then return nil, runtime_err end
    local resolved, resolve_err = runtime:resolve({
        flow_ref = ref,
        purpose = trim(purpose) ~= "" and purpose or "read",
    })
    if resolve_err then return nil, tostring(resolve_err) end
    if type(resolved) ~= "table" then return nil, "Flow runtime returned no result" end
    local canonical, canonical_err = exact_flow_ref((resolved :: Map).flow_ref)
    if canonical_err or not canonical then return nil, canonical_err or "Flow runtime returned no flow_ref" end
    if canonical.id ~= ref.id or canonical.version ~= ref.version then
        return nil, "Flow runtime changed the exact flow_ref"
    end
    local out = copy_map(resolved)
    out.flow_ref = canonical
    return out, nil
end

function M.list_destinations(): ({Map}?, any, boolean?)
    local available, availability_err = M._flow_runtime_binding_available()
    if availability_err then return nil, availability_err end
    if available ~= true then return {}, nil, false end
    local runtime, runtime_err = M._open_flow_runtime()
    if runtime_err or not runtime then return nil, runtime_err end
    local result, list_err = runtime:list({})
    if list_err then return nil, tostring(list_err) end
    local rows = type(result) == "table" and (result :: Map).destinations or nil
    if type(rows) ~= "table" then return nil, "Flow runtime returned no destinations list" end
    local out: { Map } = {}
    for _, raw in ipairs(rows :: { any }) do
        if type(raw) ~= "table" then return nil, "Flow runtime returned an invalid destination" end
        local descriptor = copy_map(raw)
        local ref, ref_err = exact_flow_ref(descriptor.flow_ref)
        if ref_err or not ref then return nil, ref_err or "Flow destination has no exact flow_ref" end
        descriptor.kind = "flow"
        descriptor.flow_ref = ref
        out[#out + 1] = descriptor
    end
    table.sort(out, function(a: Map, b: Map) return tostring(a.title or "") < tostring(b.title or "") end)
    return out, nil, true
end

-- A binding is the stored join of one Trigger, one exact Flow, and one mapping.
-- The row is the editable spec; lowerings are derived machine state keyed by binding_id.
local AUTOMATION_BINDING_TABLE = "automation_bindings"
local AUTOMATION_BINDING_KIND = "kickside.automation:automation_binding_kind"
local AUTOMATION_BINDING_META_TYPE = "kickside.automation.binding"

M.AUTOMATION_BINDING_KIND = AUTOMATION_BINDING_KIND
M.AUTOMATION_BINDING_META_TYPE = AUTOMATION_BINDING_META_TYPE

-- ─── the live type→action composition ───────────────────────────────────────
--
-- call_action's allowlist is the set of public actions the CURRENT type entry
-- declares in meta.actions, each resolved to its { contract, target }. A hub
-- install that adds an action to a type (e.g. `rewind` on a sync type) has to
-- become callable without a restart, so the allowlist is never read from a
-- one-off registry.get whose per-process view of that entry is frozen. It is
-- served from a composition rebuilt whenever the registry version changes:
-- O(types) on a version bump, O(1) per call. This mirrors build_catalog — the
-- same double-token discipline the port catalog already runs on.

type TypeActionView = {
    meta_type: string,
    actions: { [string]: automation_types.AutomationAction },
}

-- meta.type values whose contract.binding entries are automation types: the
-- provider types (AUTOMATION_TYPE) and the engine-owned binding kind
-- (AUTOMATION_BINDING_META_TYPE). An instance is callable through call_action
-- iff its impl entry carries one of these, so both populate the composition.
local COMPOSITION_META_TYPES = { AUTOMATION_TYPE, AUTOMATION_BINDING_META_TYPE }

local function build_type_composition(): ({ [string]: TypeActionView }?, any)
    local registry = dependencies.registry :: automation_types.RegistryModule
    local by_id: { [string]: TypeActionView } = {}
    for _, meta_type in ipairs(COMPOSITION_META_TYPES) do
        local entries, err = registry.find({ [".kind"] = "contract.binding", ["meta.type"] = meta_type })
        if err then return nil, err end
        for _, entry in ipairs(entries or {}) do
            local id = type(entry) == "table" and (entry :: any).id or nil
            if type(id) == "string" and id ~= "" then
                local actions: { [string]: automation_types.AutomationAction } = {}
                for _, action in ipairs(M.collect_actions(entry)) do
                    actions[action.name] = action
                end
                by_id[id :: string] = { meta_type = meta_type, actions = actions }
            end
        end
    end
    return by_id, nil
end
M._build_type_composition = build_type_composition

-- One immutable composition per registry version, gated by the same version
-- token as the port catalog. A change racing the build discards it and the
-- next caller rebuilds against the newer view, so a removed action fails
-- closed. A nil token (a runtime without current_version) builds per call —
-- the pre-composition behavior, never a clock-emulated freshness.
local composition_snapshot: Map = {}

-- rebuild_type_composition forces a fresh build and republishes the snapshot.
-- The resident watcher calls it on every registry-version change so the warm
-- view re-actualizes in real time; a caller running ahead of the watcher still
-- self-heals through type_action_view against the current version.
function M.rebuild_type_composition(): ({ [string]: TypeActionView }?, any)
    local token = registry_version_token()
    local by_id, err = build_type_composition()
    if err then return nil, err end
    if token ~= nil and registry_version_token() == token then
        composition_snapshot.token = token
        composition_snapshot.by_id = by_id
    end
    return by_id, nil
end

-- type_action_view returns the CURRENT public-action allowlist for a type impl
-- id, rebuilding only when the registry version moved. O(1) on the hot path.
local function type_action_view(impl_id: string): (TypeActionView?, any)
    local token = registry_version_token()
    if token ~= nil and composition_snapshot.token == token and composition_snapshot.by_id ~= nil then
        return (composition_snapshot.by_id :: { [string]: TypeActionView })[impl_id], nil
    end
    local by_id, err = M.rebuild_type_composition()
    if err then return nil, err end
    return by_id[impl_id], nil
end
M._type_action_view = type_action_view

-- Snapshot seam for tests: forces the next view to build afresh.
function M._reset_type_composition(): ()
    composition_snapshot.token = nil
    composition_snapshot.by_id = nil
end

local function now_rfc3339(): string
    local time_mod = dependencies.time
    local n = (time_mod :: any).now():utc()
    return tostring((n :: any):format((time_mod :: any).RFC3339))
end

local function actor_id_or_nil(): string?
    local sec = dependencies.security
    local actor = (sec :: any).actor()
    if actor then
        local id = tostring((actor :: any):id() or "")
        if id ~= "" then return id end
    end
    return nil
end

local function encode_json(value: any): string
    if type(value) == "string" then return value end
    local out, err = json.encode(type(value) == "table" and value or {})
    if err or type(out) ~= "string" then return "{}" end
    return out
end

local function is_enabled_value(raw: any): boolean
    return raw == true or raw == 1 or raw == "1" or raw == "true"
end

local function binding_db(): (Database?, string?)
    local sql_mod = dependencies.sql
    local db, err = (sql_mod :: any).get(dependencies.types.db_id())
    if err or not db then return nil, "automation bindings db unavailable: " .. tostring(err) end
    return db :: Database, nil
end

local find_trigger: ((string) -> (Map?, string?))?
local decode_persisted_binding_trigger: ((Map) -> (Map?, string?))?

local function row_to_binding(row: any): (Binding?, string?)
    local r = type(row) == "table" and (row :: Map) or {}
    if not decode_persisted_binding_trigger then return nil, "binding trigger codec unavailable" end
    local trigger, trigger_err = decode_persisted_binding_trigger(r)
    if trigger_err or not trigger then return nil, trigger_err or "binding trigger is invalid" end
    return {
        binding_id = tostring(r.binding_id or ""),
        revision = tonumber(r.revision) or 1,
        portable_key = tostring(r.portable_key or ""),
        title = tostring(r.title or ""),
        enabled = is_enabled_value(r.enabled),
        trigger = trigger,
        flow_ref = decode_json_table(r.flow_ref),
        mapping_spec = decode_json_table(r.mapping_spec),
        guard_expr = type(r.guard_expr) == "string" and r.guard_expr or nil,
        execution_policy = decode_json_table(r.execution_policy),
        authority_scope = decode_json_table(r.authority_scope),
        lowering_state = decode_json_table(r.lowering_state),
        created_by = r.created_by,
        updated_by = r.updated_by,
        created_at = r.created_at,
        updated_at = r.updated_at,
    }, nil
end

local function get_binding_row(binding_id: string): (Binding?, string?)
    local id = trim(binding_id)
    if id == "" then return nil, "binding_id is required" end
    local db, derr = binding_db()
    if not db then return nil, derr end
    local rows, qerr = db:query("SELECT * FROM " .. AUTOMATION_BINDING_TABLE .. " WHERE binding_id = $1 LIMIT 1", { id })
    db:release()
    if qerr then return nil, tostring(qerr) end
    if type(rows) ~= "table" or not (rows :: { any })[1] then return nil, nil end
    return row_to_binding((rows :: { any })[1])
end

local function normalize_binding_execution_policy(raw_policy: any): (Map?, string?)
    local policy = type(raw_policy) == "table" and copy_map(raw_policy) or {}
    local mode = trim(policy.mode)
    if mode == "" then mode = "component_owner" end
    policy.mode = mode
    if mode ~= "component_owner" then
        return nil, "execution_policy.mode must be component_owner"
    end
    for key, _ in pairs(policy) do
        if key ~= "mode" and key ~= "launch_mode" then
            return nil, "execution_policy contains undeclared field: " .. tostring(key)
        end
    end
    local launch_mode = trim(policy.launch_mode)
    if launch_mode == "" then launch_mode = "async" end
    if launch_mode ~= "async" and launch_mode ~= "sync" then
        return nil, "execution_policy.launch_mode must be async or sync"
    end
    policy.launch_mode = launch_mode
    return policy, nil
end

function M.get_binding(binding_id: string): (Binding?, any)
    local row, err = get_binding_row(binding_id)
    if err then return nil, err end
    if not row then return nil, "binding not found: " .. tostring(binding_id) end
    return row, nil
end

local function open_trigger_service(): (any?, string?)
    local contract_mod = dependencies.contract
    local def, derr = (contract_mod :: any).get("kickside.trigger:service")
    if derr or not def then return nil, "trigger service contract unavailable: " .. tostring(derr) end
    local inst, oerr = (def :: any):open()
    if oerr or not inst then return nil, "trigger service open: " .. tostring(oerr) end
    return inst, nil
end

M.SCHEDULE_TRIGGER = SCHEDULE_TRIGGER

local function normalize_trigger(raw: any): (Map?, string?)
    if type(raw) ~= "table" then return nil, "trigger is required" end
    local resolver = dependencies.trigger_resolver
    local resolution, err = (resolver :: any).resolve(raw, M.resolve_source)
    if err or type(resolution) ~= "table" then
        local detail = type(err) == "table" and (err :: Map) or {}
        return nil, tostring(detail.message or err or "invalid trigger")
    end
    local spec = (resolution :: Map).spec
    if type(spec) ~= "table" then return nil, "trigger resolver returned no normalized spec" end
    return spec :: Map, nil
end

local function trigger_catalog_id(trigger: any): string
    local spec = type(trigger) == "table" and (trigger :: Map) or {}
    local source = trim(spec.source)
    if source ~= "" then return source end
    return SCHEDULE_TRIGGER
end

local function schema_type_allows(schema_type: any, actual: string): boolean
    if schema_type == nil then return true end
    if type(schema_type) == "string" then return schema_type == actual end
    if type(schema_type) == "table" then
        for _, raw in ipairs(schema_type :: { any }) do
            if raw == actual then return true end
        end
    end
    return false
end

local function schema_type_declares(schema_type: any, actual: string): boolean
    if schema_type == nil then return false end
    return schema_type_allows(schema_type, actual)
end

local function value_kind(value: any): string
    if value == nil then return "null" end
    if type(value) == "table" then
        local max = 0
        local count = 0
        for k, _ in pairs(value :: Map) do
            if type(k) ~= "number" then return "object" end
            if k > max then max = k end
            count = count + 1
        end
        if max == count then return "array" end
        return "object"
    end
    if type(value) == "number" and value == math.floor(value) then return "integer" end
    return type(value)
end

local function value_kind_for_schema(value: any, schema_type: any): string
    local actual = value_kind(value)
    if actual == "array" and type(value) == "table" and next(value :: Map) == nil then
        if schema_type_declares(schema_type, "object") then return "object" end
        if schema_type_declares(schema_type, "array") then return "array" end
    end
    return actual
end

local validate_schema_value: (any, any, string) -> string?

local function validate_schema_object(schema: Map, value: any, path: string): string?
    if type(value) ~= "table" then return path .. " must be an object" end
    if value_kind(value) ~= "object" and next(value :: Map) ~= nil then return path .. " must be an object" end
    local props = type(schema.properties) == "table" and (schema.properties :: Map) or {}
    if type(schema.required) == "table" then
        for _, raw_key in ipairs(schema.required :: { any }) do
            local key = tostring(raw_key or "")
            if key == "" then return "invalid schema: required entries must be strings" end
            if (value :: Map)[key] == nil then return path .. "." .. key .. " is required" end
        end
    end
    for key, child_schema in pairs(props) do
        local child = (value :: Map)[key]
        if child ~= nil then
            local err = validate_schema_value(child_schema, child, path .. "." .. tostring(key))
            if err then return err end
        end
    end
    if schema.additionalProperties == false then
        for key, _ in pairs(value :: Map) do
            if props[key] == nil then return path .. "." .. tostring(key) .. " is not accepted" end
        end
    end
    return nil
end

validate_schema_value = function(schema: any, value: any, path: string): string?
    if type(schema) ~= "table" then return nil end
    local s = schema :: Map
    local expected = s.type
    local actual = value_kind_for_schema(value, expected)
    if expected ~= nil then
        local ok = schema_type_allows(expected, actual)
        if not ok and actual == "integer" then ok = schema_type_allows(expected, "number") end
        if not ok then return path .. " must match Start schema type " .. tostring(type(expected) == "table" and "union" or expected) end
    end
    if s.const ~= nil and value ~= s.const then return path .. " must match the declared const" end
    if type(s.enum) == "table" then
        local matched = false
        for _, option in ipairs(s.enum :: { any }) do
            if value == option then matched = true; break end
        end
        if not matched then return path .. " must be one of the declared options" end
    end
    if actual == "object" or schema_type_declares(expected, "object") then
        local err = validate_schema_object(s, value, path)
        if err then return err end
    elseif actual == "array" or schema_type_declares(expected, "array") then
        if actual ~= "array" then return path .. " must be an array" end
        if type(s.items) == "table" then
            for idx, item in ipairs(value :: { any }) do
                local err = validate_schema_value(s.items, item, path .. "[" .. tostring(idx) .. "]")
                if err then return err end
            end
        end
    end
    return nil
end

local function schema_sample(schema: any): any
    local s = type(schema) == "table" and (schema :: Map) or {}
    local typ = s.type
    if type(typ) == "table" then typ = (typ :: { any })[1] end
    if typ == "object" or type(s.properties) == "table" then
        local out: Map = {}
        if type(s.properties) == "table" then
            for key, child in pairs(s.properties :: Map) do out[tostring(key)] = schema_sample(child) end
        end
        return out
    end
    if typ == "array" then return { schema_sample(s.items) } end
    if typ == "integer" or typ == "number" then return 1 end
    if typ == "boolean" then return true end
    if s.const ~= nil then return s.const end
    if type(s.enum) == "table" and (s.enum :: { any })[1] ~= nil then return (s.enum :: { any })[1] end
    return "value"
end

local function apply_mapping_sample(mapping_spec: any, trigger_id: string): (any?, string?)
    local point: Map? = nil
    if find_trigger then point = select(1, find_trigger(trigger_id)) end
    if not point then return nil, "trigger not found: " .. trigger_id end
    local input = schema_sample((point :: Map).context_schema)
    if type(input) ~= "table" then input = {} end
    ; (input :: Map).trigger_id = trigger_id
    ; (input :: Map).binding_id = "binding-sample"
    ; (input :: Map).occurred_at = "2026-01-01T00:00:00Z"
    ; (input :: Map).event_type = trim((point :: Map).kind)

    local mapping = type(mapping_spec) == "table" and (mapping_spec :: Map) or {}
    if next(mapping) == nil then return input, nil end
    local mode = trim(mapping.mode)
    if mode == "" then mode = "expr" end
    if mode ~= "expr" and mode ~= "expr_generated" then return nil, "binding mapping mode unsupported: " .. mode end
    local source = trim(mapping.expr)
    if source == "" then return nil, "binding mapping expr is required" end
    local expr_mod = dependencies.expr
    local out, err = (expr_mod :: any).eval(source, { input = input })
    if err then return nil, "binding mapping expr: " .. tostring(err) end
    return type(out) == "table" and out or { value = out }, nil
end

local function validate_binding_mapping(spec: BindingSpec, input_schema: any): string?
    if type(input_schema) ~= "table" then return nil end
    local mapped, merr = apply_mapping_sample(spec.mapping_spec, trigger_catalog_id(spec.trigger))
    if merr then return merr end
    local verr = validate_schema_value(input_schema, mapped, "mapping output")
    if verr then return "binding mapping output does not match destination input schema: " .. verr end
    return nil
end

local function validate_binding(input: any): (BindingSpec?, string?)
    if type(input) ~= "table" then return nil, "binding spec must be a table" end
    local raw = input :: Map
    local portable_key = trim(raw.portable_key)
    if portable_key == "" then return nil, "portable_key is required" end
    local title = trim(raw.title)
    if title == "" then return nil, "title is required" end
    local trigger, trigger_err = normalize_trigger(raw.trigger)
    if trigger_err or not trigger then return nil, trigger_err or "invalid trigger" end
    local flow_ref, flow_ref_err = exact_flow_ref(raw.flow_ref)
    if flow_ref_err or not flow_ref then return nil, flow_ref_err end
    local execution_policy, policy_err = normalize_binding_execution_policy(raw.execution_policy)
    if policy_err or not execution_policy then return nil, policy_err or "invalid execution_policy" end
    local authority_scope: Map = {}
    if raw.authority_scope ~= nil then
        if type(raw.authority_scope) ~= "table" then return nil, "authority_scope must be an object" end
        for key, _ in pairs(raw.authority_scope :: Map) do
            if key ~= "app_scope" then return nil, "authority_scope contains undeclared field: " .. tostring(key) end
        end
        if (raw.authority_scope :: Map).app_scope ~= nil then
            local app_scope = trim((raw.authority_scope :: Map).app_scope)
            if app_scope == "" then return nil, "authority_scope.app_scope must be a non-empty string" end
            authority_scope.app_scope = app_scope
        end
    end
    local spec: BindingSpec = {
        portable_key = portable_key,
        title = title,
        enabled = raw.enabled == true,
        trigger = trigger,
        flow_ref = flow_ref,
        mapping_spec = type(raw.mapping_spec) == "table" and (raw.mapping_spec :: Map) or {},
        guard_expr = type(raw.guard_expr) == "string" and trim(raw.guard_expr) ~= "" and trim(raw.guard_expr) or nil,
        execution_policy = execution_policy,
        authority_scope = authority_scope,
    }
    return spec, nil
end

local function register_binding_component(binding_id: string, title: string, parent_id: any, identity: any, flow_ref: Map): string?
    local component = dependencies.component :: automation_types.ComponentModule
    local svc, svc_err = component.get_service()
    if not svc then return "component service: " .. tostring(svc_err) end
    local meta: Map = {
        title = title,
        icon = "tabler:plug-connected",
        class = "automation_binding",
        flow_ref = copy_map(flow_ref),
    }
    local req: Map = {
        component_id = binding_id,
        impl_id = AUTOMATION_BINDING_KIND,
        private_context = { component_id = binding_id, [EXECUTION_IDENTITY_KEY] = identity },
        meta = meta,
    }
    if type(parent_id) == "string" and parent_id ~= "" then req.parent_id = parent_id end
    local result, reg_err = svc:register(req)
    if reg_err or not result or not result.component_id then
        return "binding component registration failed: " .. tostring(reg_err)
    end
    return nil
end

-- Component registration crosses the SQL boundary, so every later create failure
-- tears the shell down through its normal deletable lifecycle. That lifecycle is
-- deliberately idempotent when the binding row never committed, and also removes
-- the binding row if a commit outcome was ambiguous.
local function abandon_binding_component(binding_id: string, reason: any): string
    local message = tostring(reason)
    local component = dependencies.component :: automation_types.ComponentModule
    local svc, svc_err = component.get_service()
    if not svc then return message .. "; binding component cleanup unavailable: " .. tostring(svc_err) end
    local deleted, delete_err = svc:delete({ component_id = binding_id })
    if delete_err or not deleted or (type(deleted) == "table" and (deleted :: Map).success == false) then
        local detail = delete_err or (type(deleted) == "table" and (deleted :: Map).error) or "no result"
        return message .. "; binding component cleanup failed: " .. tostring(detail)
    end
    return message
end

local function binding_id_new(): string
    local uuid_mod = dependencies.uuid
    return tostring((uuid_mod :: any).v7())
end

function M.create_binding(input: any, parent_id: string?): (Binding?, any)
    local spec, verr = validate_binding(input)
    if not spec then return nil, verr end
    local resolution, resolution_err = M.resolve_flow_ref(spec.flow_ref, "create")
    if resolution_err or not resolution then return nil, resolution_err or "Flow runtime resolution failed" end
    spec.flow_ref = (resolution :: Map).flow_ref :: Map
    local schema_err = validate_binding_mapping(spec :: BindingSpec, (resolution :: Map).input_schema)
    if schema_err then return nil, schema_err end
    local binding_id = trim((type(input) == "table" and (input :: Map).binding_id) or "")
    if binding_id == "" then binding_id = binding_id_new() end
    local identity, identity_err = capture_execution_identity()
    if identity_err or not identity then return nil, identity_err or "could not capture binding execution identity" end
    local cerr = register_binding_component(binding_id, tostring(spec.title), parent_id, identity, spec.flow_ref)
    if cerr then return nil, cerr end

    local created_at = now_rfc3339()
    local actor_id = actor_id_or_nil()
    local lowering_state: Map = {}
    local db, derr = binding_db()
    if not db then return nil, abandon_binding_component(binding_id, derr) end
    local tx, tx_err = db:begin()
    if tx_err or not tx then
        db:release()
        return nil, abandon_binding_component(binding_id, "binding transaction begin failed: " .. tostring(tx_err))
    end
    local _, ierr = tx:execute([[
        INSERT INTO automation_bindings
            (binding_id, revision, portable_key, title, enabled, trigger_id, trigger_config,
             flow_ref, mapping_spec, guard_expr, execution_policy, authority_scope,
             lowering_state, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
    ]], {
        binding_id,
        1,
        spec.portable_key,
        spec.title,
        false,
        trigger_catalog_id(spec.trigger),
        encode_json(spec.trigger),
        encode_json(spec.flow_ref),
        encode_json(spec.mapping_spec),
        spec.guard_expr,
        encode_json(spec.execution_policy),
        encode_json(spec.authority_scope),
        encode_json(lowering_state),
        actor_id,
        actor_id,
        created_at,
        created_at,
    })
    if ierr then
        tx:rollback()
        db:release()
        return nil, abandon_binding_component(binding_id, ierr)
    end
    local _, commit_err = tx:commit()
    if commit_err then
        tx:rollback()
        db:release()
        return nil, abandon_binding_component(binding_id, "binding transaction commit failed: " .. tostring(commit_err))
    end
    db:release()
    local row: Binding = {
        binding_id = binding_id,
        revision = 1,
        portable_key = spec.portable_key,
        title = spec.title,
        enabled = false,
        trigger = spec.trigger,
        flow_ref = spec.flow_ref,
        mapping_spec = spec.mapping_spec,
        guard_expr = spec.guard_expr,
        execution_policy = spec.execution_policy,
        authority_scope = spec.authority_scope,
        lowering_state = lowering_state,
        created_by = actor_id,
        updated_by = actor_id,
        created_at = created_at,
        updated_at = created_at,
    }
    if spec.enabled == true then
        local enabled, enable_err = M.enable_binding(binding_id)
        if enable_err or type(enabled) ~= "table" then return nil, enable_err or "binding enable failed" end
        return enabled :: Binding, nil
    end
    return row, nil
end

local function persist_binding_enabled(binding_id: string, enabled: boolean, lowering_state: any): (Binding?, any)
    local db, derr = binding_db()
    if not db then return nil, derr end
    local updated_at = now_rfc3339()
    local _, uerr = db:execute(
        "UPDATE " .. AUTOMATION_BINDING_TABLE .. " SET enabled = $1, lowering_state = $2, updated_at = $3 WHERE binding_id = $4",
        { enabled, encode_json(lowering_state), updated_at, binding_id })
    db:release()
    if uerr then return nil, tostring(uerr) end
    local row, rerr = get_binding_row(binding_id)
    if rerr then return nil, rerr end
    if not row then return nil, "binding not found: " .. binding_id end
    return row, nil
end

local function lowering_is_attached(state: any): boolean
    return type(state) == "table" and trim((state :: Map).mode) ~= ""
end

local function binding_replacement(state: any): Map?
    if type(state) ~= "table" then return nil end
    local replacement = (state :: Map).replacement
    if type(replacement) ~= "table" then return nil end
    return replacement :: Map
end

-- Runtime read model. The owner of a transition publishes what only it knows
-- into the binding's public meta as that transition commits, so listing projects
-- the value from the meta it already reads instead of describing each row's
-- machine. The patch is projected through the kind's declared
-- public_state_schema: a field the kind does not publish is not read-model state.
--
-- The published state is derived from a transition that has already committed,
-- and delivery side effects are not repeatable, so a publish failure is reported
-- and never turns a completed transition into a failed one.
local function publish_binding_runtime(binding_id: any, patch: Map): ()
    local id = trim(binding_id)
    if id == "" then return end
    local component = dependencies.component :: automation_types.ComponentModule
    local registry = dependencies.registry :: automation_types.RegistryModule
    local logger_mod = dependencies.logger :: LoggerModule

    local schema, schema_err = public_state_schema_for_impl(registry, AUTOMATION_BINDING_KIND)
    if schema_err then
        logger_mod:named("automations.binding_runtime"):warn("binding public state schema unavailable", {
            binding_id = id,
            error = tostring(schema_err),
        })
        return
    end
    local fields, project_err = project_public_state(schema, patch)
    if project_err or not fields then
        logger_mod:named("automations.binding_runtime"):warn("binding runtime state is not publishable", {
            binding_id = id,
            error = tostring(project_err or "public state projection returned nothing"),
        })
        return
    end
    if next(fields :: Map) == nil then return end

    local ok, set_err = (component :: any).set_meta(id, fields)
    if not ok then
        logger_mod:named("automations.binding_runtime"):warn("binding runtime state publish failed", {
            binding_id = id,
            error = tostring(set_err or "component.set_meta failed"),
        })
    end
end

function M.enable_binding(binding_id: string): (Binding?, any)
    local binding, err = get_binding_row(binding_id)
    if err then return nil, err end
    if not binding then return nil, "binding not found: " .. tostring(binding_id) end
    if binding_replacement(binding.lowering_state) then return nil, "binding replacement is in progress" end
    local _, policy_err = normalize_binding_execution_policy(binding.execution_policy)
    if policy_err then return nil, policy_err end
    local lowering_state = binding.lowering_state
    local attached = false
    if not lowering_is_attached(lowering_state) then
        local lowered, lerr = M._attach_binding_lowering(binding, true)
        if lerr or not lowered then return nil, lerr or "binding lowering failed" end
        lowering_state = lowered
        attached = true
    end
    local persisted, persist_err = persist_binding_enabled(binding.binding_id, true, lowering_state)
    if persist_err and attached then
        local _, detach_err = M._detach_binding_lowering(lowering_state)
        if detach_err then
            return nil, tostring(persist_err) .. "; binding lowering cleanup failed: " .. tostring(detach_err)
        end
    end
    return persisted, persist_err
end

function M.disable_binding(binding_id: string): (Binding?, any)
    local binding, err = get_binding_row(binding_id)
    if err then return nil, err end
    if not binding then return nil, "binding not found: " .. tostring(binding_id) end
    if binding_replacement(binding.lowering_state) then return nil, "binding replacement is in progress" end
    local lowering_state = binding.lowering_state
    if not lowering_is_attached(lowering_state) then
        local unlowered, unlowered_err = persist_binding_enabled(binding.binding_id, false, {})
        if unlowered then publish_binding_runtime(binding.binding_id, { next_run_at = "" }) end
        return unlowered, unlowered_err
    end

    -- Disable the authoritative row before touching the external trigger. If the
    -- detach or the final state clear fails, the retained lowering_state is a
    -- durable retry record and dispatch remains gated by enabled=false.
    local disabled, disable_err = persist_binding_enabled(binding.binding_id, false, lowering_state)
    if disable_err or not disabled then return nil, disable_err or "binding disable persistence failed" end
    local _, detach_err = M._detach_binding_lowering(lowering_state)
    if detach_err then
        return nil, "binding disabled but lowering cleanup failed: " .. tostring(detach_err)
    end
    local cleared, clear_err = persist_binding_enabled(binding.binding_id, false, {})
    if clear_err or not cleared then
        return nil, "binding disabled and lowering detached but cleanup state update failed: "
            .. tostring(clear_err or "binding not found")
    end
    -- The machine is detached: there is no run to be next.
    publish_binding_runtime(binding.binding_id, { next_run_at = "" })
    return cleared, nil
end

local function binding_component_id(): (string?, any)
    local ctx_mod = dependencies.ctx
    local id = trim((ctx_mod :: any).get("component_id"))
    if id == "" then return nil, "binding component_id is required" end
    return id, nil
end

local function values_equal(left: any, right: any): boolean
    if type(left) ~= type(right) then return false end
    if type(left) ~= "table" then return left == right end
    for key, value in pairs(left :: Map) do
        if not values_equal(value, (right :: Map)[key]) then return false end
    end
    for key, _ in pairs(right :: Map) do
        if (left :: Map)[key] == nil then return false end
    end
    return true
end

local function binding_matches_spec(binding: Binding, spec: BindingSpec, desired_enabled: boolean): boolean
    return binding.portable_key == spec.portable_key
        and binding.title == spec.title
        and binding.guard_expr == spec.guard_expr
        and values_equal(binding.trigger, spec.trigger)
        and values_equal(binding.flow_ref, spec.flow_ref)
        and values_equal(binding.mapping_spec, spec.mapping_spec)
        and values_equal(binding.execution_policy, spec.execution_policy)
        and values_equal(binding.authority_scope, spec.authority_scope)
        and desired_enabled == (spec.enabled == true)
end

local function rows_affected(result: any): number?
    if type(result) == "number" then return result end
    if type(result) ~= "table" then return nil end
    return tonumber((result :: Map).rows_affected or (result :: Map).affected_rows)
end

local function persist_replacement_state(binding_id: string, revision: number, enabled: boolean, lowering_state: any): (Binding?, any)
    local db, derr = binding_db()
    if not db then return nil, derr end
    local result, uerr = db:execute(
        "UPDATE " .. AUTOMATION_BINDING_TABLE
            .. " SET enabled = $1, lowering_state = $2, updated_by = $3, updated_at = $4"
            .. " WHERE binding_id = $5 AND revision = $6",
        { enabled, encode_json(lowering_state), actor_id_or_nil(), now_rfc3339(), binding_id, revision })
    db:release()
    if uerr then return nil, tostring(uerr) end
    if rows_affected(result) ~= 1 then return nil, "binding revision conflict" end
    local binding, read_err = get_binding_row(binding_id)
    if read_err then return nil, read_err end
    if not binding then return nil, "binding not found: " .. binding_id end
    return binding, nil
end

local function cas_binding_spec(binding: Binding, spec: BindingSpec, marker: Map): (Binding?, any)
    local db, derr = binding_db()
    if not db then return nil, derr end
    local result, uerr = db:execute([[
        UPDATE automation_bindings
           SET title = $1, enabled = $2, trigger_id = $3, trigger_config = $4,
               flow_ref = $5, mapping_spec = $6, guard_expr = $7, execution_policy = $8,
               authority_scope = $9, lowering_state = $10,
               updated_by = $11, updated_at = $12, revision = revision + 1
         WHERE binding_id = $13 AND revision = $14
    ]], {
        spec.title,
        false,
        trigger_catalog_id(spec.trigger),
        encode_json(spec.trigger),
        encode_json(spec.flow_ref),
        encode_json(spec.mapping_spec),
        spec.guard_expr,
        encode_json(spec.execution_policy),
        encode_json(spec.authority_scope),
        encode_json({ replacement = marker }),
        actor_id_or_nil(),
        now_rfc3339(),
        binding.binding_id,
        binding.revision,
    })
    db:release()
    if uerr then return nil, tostring(uerr) end
    if rows_affected(result) ~= 1 then return nil, "binding revision conflict" end
    local updated, read_err = get_binding_row(binding.binding_id)
    if read_err then return nil, read_err end
    if not updated then return nil, "binding not found: " .. tostring(binding.binding_id) end
    return updated, nil
end

local function phase_from_trigger(trigger: Map, progress: Map): (string?, string?)
    local trigger_phase = trim(trigger.phase)
    if trigger_phase == "paused" then return "paused", nil end
    local progress_phase = trim(progress.phase)
    if progress_phase ~= "" then return progress_phase, nil end
    if trigger_phase ~= "" then return trigger_phase, nil end
    return nil, "trigger phase is missing"
end

local function lowering_from_private_trigger(binding_id: string): (Map?, any)
    local component = dependencies.component :: automation_types.ComponentModule
    local private, err = (component :: any).get_private_context(binding_id)
    if err then return nil, err end
    if type(private) ~= "table" then return nil, nil end
    local trigger = (private :: Map)[TRIGGER_STATE_KEY]
    if type(trigger) ~= "table" or trim((trigger :: Map).mode) == "" then return nil, nil end
    local t = trigger :: Map
    local progress = type((private :: Map)[TRIGGER_PROGRESS_KEY]) == "table"
        and ((private :: Map)[TRIGGER_PROGRESS_KEY] :: Map) or {}
    local phase, phase_err = phase_from_trigger(t, progress)
    if not phase then return nil, phase_err end
    return {
        v = 1,
        component_id = binding_id,
        mode = trim(t.mode),
        phase = phase,
        spec = type(t.spec) == "table" and t.spec or {},
        registration = type(t.registration) == "table" and t.registration or {},
        rollback = {},
        cursor = progress.cursor,
    }, nil
end

local function update_binding_component_meta(binding: Binding): string?
    local component = dependencies.component :: automation_types.ComponentModule
    local ok, err = (component :: any).set_meta(binding.binding_id, {
        title = binding.title,
        flow_ref = copy_map(binding.flow_ref),
    })
    if not ok then return tostring(err or "component metadata update failed") end
    return nil
end

local function continue_binding_replacement(binding: Binding): (Binding?, any)
    local marker = binding_replacement(binding.lowering_state)
    if not marker then return nil, "binding replacement state is missing" end
    local revision = tonumber(binding.revision)
    if not revision then return nil, "binding revision is missing" end

    if marker.phase == "detach_old" then
        local old_lowering = type(marker.old_lowering) == "table" and marker.old_lowering or {}
        if lowering_is_attached(old_lowering) then
            local _, detach_err = M._detach_binding_lowering(old_lowering)
            if detach_err then
                return nil, "binding replacement staged disabled; old lowering cleanup failed: " .. tostring(detach_err)
            end
        end
        marker = copy_map(marker)
        marker.phase = "attach_new"
        local persisted, persist_err = persist_replacement_state(binding.binding_id, revision, false, { replacement = marker })
        if persist_err or not persisted then
            return nil, "old lowering detached but replacement progress persistence failed: " .. tostring(persist_err)
        end
        binding = persisted
    end

    marker = binding_replacement(binding.lowering_state)
    if marker and marker.phase == "attach_new" then
        local new_lowering: any = nil
        if marker.desired_enabled == true then
            if marker.attach_attempted == true then
                local recovered, recover_err = lowering_from_private_trigger(binding.binding_id)
                if recover_err then
                    return nil, "replacement lowering recovery failed: " .. tostring(recover_err)
                end
                new_lowering = recovered
            end
            if not new_lowering then
                marker = copy_map(marker)
                marker.attach_attempted = true
                local attempted, attempt_err = persist_replacement_state(binding.binding_id, revision, false, { replacement = marker })
                if attempt_err or not attempted then
                    return nil, "replacement attach intent persistence failed: " .. tostring(attempt_err)
                end
                binding = attempted
                local lowered, attach_err = M._attach_binding_lowering(binding, marker.desired_enabled == true)
                if attach_err or not lowered then return nil, "replacement lowering failed: " .. tostring(attach_err) end
                new_lowering = lowered
            end
        else
            new_lowering = {}
        end
        marker = copy_map(marker)
        marker.phase = "finalize"
        marker.new_lowering = new_lowering
        local persisted, persist_err = persist_replacement_state(binding.binding_id, revision, false, { replacement = marker })
        if persist_err or not persisted then
            if lowering_is_attached(new_lowering) then
                local _, cleanup_err = M._detach_binding_lowering(new_lowering)
                if cleanup_err then
                    return nil, "replacement lowering state persistence failed: " .. tostring(persist_err)
                        .. "; new lowering cleanup failed: " .. tostring(cleanup_err)
                end
            end
            return nil, "replacement lowering state persistence failed: " .. tostring(persist_err)
        end
        binding = persisted
    end

    marker = binding_replacement(binding.lowering_state)
    if not marker or marker.phase ~= "finalize" then return nil, "invalid binding replacement phase" end
    local meta_err = update_binding_component_meta(binding)
    if meta_err then return nil, "binding replacement metadata update failed: " .. meta_err end
    local final_lowering = type(marker.new_lowering) == "table" and marker.new_lowering or {}
    return persist_replacement_state(binding.binding_id, revision, marker.desired_enabled == true, final_lowering)
end

-- replace_binding performs one revisioned spec cutover. Validation and exact
-- Flow runtime resolution happen before the SQL CAS, so every pre-cutover failure
-- leaves the old binding active. The CAS stages the new row disabled; durable
-- phases then detach old derived state, attach the new lowering, update only the
-- shell metadata, and finally restore the requested enabled state. Retrying the
-- same replacement resumes those phases without changing component identity or
-- its frozen execution identity.
function M.replace_binding(args: any): (Binding?, any)
    if type(args) ~= "table" then return nil, "binding replacement spec must be a table" end
    local input = args :: Map
    local expected_revision = input.expected_revision
    if type(expected_revision) ~= "number" or expected_revision < 1 or expected_revision ~= math.floor(expected_revision) then
        return nil, "expected_revision must be a positive integer"
    end
    local requested_id = trim(input.binding_id)
    local ctx_mod = dependencies.ctx
    local contextual_id = trim((ctx_mod :: any).get("component_id"))
    if contextual_id ~= "" and requested_id ~= "" and requested_id ~= contextual_id then
        return nil, "binding_id does not match the opened binding component"
    end
    local binding_id = contextual_id ~= "" and contextual_id or requested_id
    if binding_id == "" then
        local id, id_err = binding_component_id()
        if id_err or not id then return nil, id_err end
        binding_id = id
    end
    local current, read_err = get_binding_row(binding_id)
    if read_err then return nil, read_err end
    if not current then return nil, "binding not found: " .. binding_id end

    local spec, validation_err = validate_binding(input)
    if validation_err or not spec then return nil, validation_err end
    if spec.portable_key ~= current.portable_key then return nil, "portable_key is immutable" end
    local resolution, resolution_err = M.resolve_flow_ref(spec.flow_ref, "replace")
    if resolution_err or not resolution then return nil, resolution_err or "Flow runtime resolution failed" end
    spec.flow_ref = (resolution :: Map).flow_ref :: Map
    local mapping_err = validate_binding_mapping(spec :: BindingSpec, (resolution :: Map).input_schema)
    if mapping_err then return nil, mapping_err end

    local marker = binding_replacement(current.lowering_state)
    if marker then
        local original_revision = tonumber(marker.expected_revision)
        if expected_revision ~= original_revision and expected_revision ~= current.revision then
            return nil, "binding revision conflict"
        end
        if not binding_matches_spec(current, spec, marker.desired_enabled == true) then
            return nil, "a different binding replacement is in progress"
        end
        return continue_binding_replacement(current)
    end
    if current.revision ~= expected_revision then return nil, "binding revision conflict" end

    marker = {
        v = 1,
        phase = "detach_old",
        expected_revision = expected_revision,
        desired_enabled = spec.enabled == true,
        old_lowering = current.lowering_state,
    }
    local staged, stage_err = cas_binding_spec(current, spec :: BindingSpec, marker)
    if stage_err or not staged then return nil, stage_err or "binding replacement staging failed" end
    return continue_binding_replacement(staged)
end

-- Binding artifact shells use the pausable action contract. Their durable enabled
-- state and cron ownership live in the binding row, so controls delegate to the
-- lifecycle rather than writing component metadata.
function M.pause_binding(_args: any): (Map?, any)
    local id, id_err = binding_component_id()
    if id_err or not id then return nil, id_err end
    local binding, err = M.disable_binding(id)
    if err or not binding then return nil, err or "binding pause failed" end
    return { success = true, id = id, enabled = binding.enabled }, nil
end

function M.resume_binding(_args: any): (Map?, any)
    local id, id_err = binding_component_id()
    if id_err or not id then return nil, id_err end
    local binding, err = M.enable_binding(id)
    if err or not binding then return nil, err or "binding resume failed" end
    return { success = true, id = id, enabled = binding.enabled }, nil
end

-- Generic reconfigure remains the lifecycle switch used by the existing PUT
-- surface. Structural edits use the explicit revisioned replace action above,
-- which cannot be confused with pause/resume.
function M.reconfigure_binding(args: any): (Map?, any)
    local input = type(args) == "table" and (args :: Map) or nil
    if not input or type(input.enabled) ~= "boolean" then
        return nil, "binding reconfigure requires enabled boolean"
    end
    for key, _ in pairs(input) do
        if key ~= "enabled" then return nil, "binding reconfigure only supports enabled; change the trigger, filter, schedule or destination with the replace action" end
    end
    if input.enabled then
        local resumed, resume_err = M.resume_binding({})
        if resume_err or type(resumed) ~= "table" then return nil, resume_err or "binding resume failed" end
        return resumed :: Map, nil
    end
    local paused, pause_err = M.pause_binding({})
    if pause_err or type(paused) ~= "table" then return nil, pause_err or "binding pause failed" end
    return paused :: Map, nil
end

local function binding_public_state(binding: Binding): Map
    local enabled = binding.enabled == true
    local status = "paused"
    if enabled then status = "idle" end
    return { enabled = enabled, status = status }
end

-- A destination reference is opaque by contract: only its provider can turn one
-- into words. An absent provider has nothing to say about it; a present provider
-- that cannot resolve the reference is reporting a destination the operator has
-- to repair, and that stays visible rather than reading as an unnamed id.
local function destination_name(flow_ref: any): (string?, boolean?)
    local available, _availability_err = M._flow_runtime_binding_available()
    if available ~= true then return nil, nil end
    local resolved, _resolve_err = M.resolve_flow_ref(flow_ref, "read")
    if type(resolved) ~= "table" then return nil, true end
    local title = trim((resolved :: Map).title)
    if title == "" then return nil, nil end
    return title, nil
end

local function binding_config(binding: Binding): Map
    return {
        revision = binding.revision,
        portable_key = binding.portable_key,
        title = binding.title,
        enabled = binding.enabled,
        trigger = binding.trigger,
        flow_ref = binding.flow_ref,
        mapping_spec = binding.mapping_spec,
        guard_expr = binding.guard_expr,
        execution_policy = binding.execution_policy,
        authority_scope = binding.authority_scope,
    }
end

-- delete_binding_data removes the binding's durable side data. It is deliberately
-- only reached from the binding kind's deletable contract: component teardown
-- invokes that contract before unregistering the component, so a failed schedule
-- detach retains both the binding row and component for a safe retry.
local function delete_binding_data(id: string): (Map?, any)
    local existing, read_err = get_binding_row(id)
    if read_err then return nil, read_err end
    if existing then
        local _, derr = M.disable_binding(id)
        if derr then return nil, derr end
    end
    local db, dberr = binding_db()
    if not db then return nil, dberr end
    local _, xerr = db:execute("DELETE FROM " .. AUTOMATION_BINDING_TABLE .. " WHERE binding_id = $1", { id })
    db:release()
    if xerr then return nil, tostring(xerr) end

    return { success = true, id = id }, nil
end

-- delete_binding is both the public binding-delete entry point and the binding
-- kind's deletable handler. An explicit id enters the standard component
-- teardown path. The component service invokes this same function with the
-- component context and no id; only that lifecycle invocation removes the
-- binding row and its lowered resources. Keeping those roles on one path means
-- direct deletion, HTTP deletion, and lifecycle reaping have identical cleanup.
function M.delete_binding(binding_id: any): (Map?, any)
    local id = trim(binding_id)
    if id == "" then
        local ctx_mod = dependencies.ctx
        id = trim((ctx_mod :: any).get("component_id"))
        if id == "" then return nil, "binding component_id is required" end
        return delete_binding_data(id)
    end

    local component = dependencies.component :: automation_types.ComponentModule
    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local deleted, cerr = svc:delete({ component_id = id })
    if cerr or not deleted or (type(deleted) == "table" and (deleted :: Map).success == false) then
        return nil, "delete binding component: " .. tostring(cerr or
            (type(deleted) == "table" and (deleted :: Map).error) or "no result")
    end
    return { success = true, id = id }, nil
end

function M.list_bindings(opts: any): ({ Map }?, any)
    local options = type(opts) == "table" and (opts :: Map) or {}
    local db, derr = binding_db()
    if not db then return nil, derr end
    local rows: any
    local qerr: any
    if trim(options.source) ~= "" then
        rows, qerr = db:query(
            "SELECT * FROM " .. AUTOMATION_BINDING_TABLE .. " WHERE trigger_id = $1 ORDER BY updated_at DESC",
            { trim(options.source) })
    else
        rows, qerr = db:query("SELECT * FROM " .. AUTOMATION_BINDING_TABLE .. " ORDER BY updated_at DESC", {})
    end
    db:release()
    if qerr then return nil, tostring(qerr) end
    local out: { Map } = {}
    for _, row in ipairs(type(rows) == "table" and rows or {}) do
        local binding, decode_err = row_to_binding(row)
        if decode_err or not binding then return nil, decode_err or "binding trigger is invalid" end
        if options.enabled == nil or binding.enabled == options.enabled then out[#out + 1] = binding end
    end
    return out, nil
end

find_trigger = function(trigger_id: string): (Map?, string?)
    local points, err = M.list_triggers()
    if err then return nil, tostring(err) end
    for _, point in ipairs(points or {}) do
        if trim((point :: Map).id) == trigger_id then return point :: Map, nil end
    end
    return nil, "trigger not found: " .. trigger_id
end

-- Persisted bindings store the canonical Trigger Spec v2. Reads never infer or
-- repair another shape: migrations own storage transitions, while this boundary
-- fails closed if a row does not satisfy the current contract.
decode_persisted_binding_trigger = function(row: Map): (Map?, string?)
    local stored = decode_json_table(row.trigger_config)
    if stored.v ~= 2 then return nil, "stored binding trigger must be canonical Trigger Spec v2" end
    return stored, nil
end

M._attach_binding_lowering = function(binding: any, enabled: any): (any?, any)
    local b = type(binding) == "table" and (binding :: Map) or {}
    local spec = type(b.trigger) == "table" and (b.trigger :: Map) or nil
    if not spec then return nil, "binding trigger is missing" end
    if type(enabled) ~= "boolean" then return nil, "binding lowering enabled state is required" end

    local service, oerr = open_trigger_service()
    if oerr or not service then return nil, oerr end
    local rollback: { Map } = {}
    local recorder = {
        rollback = function(target_id: string, args: Map?)
            if trim(target_id) ~= "" then
                rollback[#rollback + 1] = { target = target_id, args = type(args) == "table" and args or {} }
            end
        end,
    }
    local installed, ierr = (service :: any):install({
        spec = spec,
        consumer = {
            component_id = b.binding_id,
            drop_event_type = TRIGGER_DROPPED_EVENT,
            description = "Automation: " .. tostring(b.title or b.binding_id),
        },
        enabled = enabled,
    })
    if ierr then
        M.replay(rollback)
        return nil, tostring(ierr)
    end
    local out = type(installed) == "table" and (installed :: Map) or {}
    if out.success ~= true then
        local e = type(out.error) == "table" and (out.error :: Map) or {}
        M.replay(rollback)
        return nil, tostring(e.message or out.error or "trigger service install failed")
    end
    local teardown_err = M.record_trigger_teardown(recorder, b.binding_id, out.teardown)
    if teardown_err then return nil, teardown_err end
    local trigger = type(out.trigger) == "table" and (out.trigger :: Map) or {}
    local progress = type(out.trigger_progress) == "table" and (out.trigger_progress :: Map) or {}
    local phase, phase_err = phase_from_trigger(trigger, progress)
    if not phase then
        M.replay(rollback)
        return nil, phase_err
    end
    local _, werr = M.write_trigger_install_state(tostring(b.binding_id or ""), trigger, progress)
    if werr then
        M.replay(rollback)
        return nil, "write binding trigger state: " .. tostring(werr)
    end
    -- The machine reports its next run only while it is being scheduled, and a
    -- freshly attached machine has delivered nothing yet. A mode that schedules
    -- nothing (watch) clears the field rather than leaving a past run standing.
    local runtime_patch: Map = {
        next_run_at = trim(out.next_run_at),
        last_error = "",
    }
    local trigger_spec: Map = (type(trigger.spec) == "table" and (trigger.spec :: Map) or spec) :: Map
    if trim(trigger.mode) == "poll" and type(trigger_spec.poll) == "table" then
        local poll = trigger_spec.poll :: Map
        runtime_patch.max_pull_pages_per_run = poll.max_pages_per_run
        runtime_patch.max_items_per_run = poll.max_items_per_run
        runtime_patch.accepted_count = 0
        runtime_patch.last_run_accepted = 0
    end
    publish_binding_runtime(b.binding_id, runtime_patch)
    return {
        v = 1,
        component_id = b.binding_id,
        mode = trim(trigger.mode),
        phase = phase,
        spec = type(trigger.spec) == "table" and trigger.spec or spec,
        registration = type(trigger.registration) == "table" and trigger.registration or {},
        cursor = progress.cursor,
    }, nil
end

M._detach_binding_lowering = function(lowering_state: any): (any?, any)
    local state = type(lowering_state) == "table" and (lowering_state :: Map) or {}
    local mode = trim(state.mode)
    if mode == "" then return { success = true }, nil end
    local component_id = trim(state.component_id)
    if component_id == "" then return nil, "binding lowering component_id missing" end
    local service, oerr = open_trigger_service()
    if oerr or not service then return nil, oerr end
    local detached, derr = (service :: any):uninstall({ component_id = component_id })
    if derr then return nil, tostring(derr) end
    local d = type(detached) == "table" and (detached :: Map) or {}
    if d.success == false then
        local e = type(d.error) == "table" and (d.error :: Map) or {}
        return nil, tostring(e.message or d.error or "detach failed")
    end
    return { success = true }, nil
end

local function mapping_input_context(
    binding: Binding,
    raw_context: any,
    args: Map,
    binding_ref: Map
): Map
    local input = type(raw_context) == "table" and copy_map(raw_context) or {}
    input.trigger_id = input.trigger_id or trigger_catalog_id(binding.trigger)
    input.automation_ref = copy_map(binding_ref)
    input.occurred_at = input.occurred_at or args.occurred_at
    input.event_type = input.event_type or args.event_type
    input.dedup_key = input.dedup_key or args.dedup_key
    input.trace_context = input.trace_context or args.trace_context
    return input
end

local function eval_expr(source: string, env: Map): (any?, any)
    local expr_mod = dependencies.expr
    return (expr_mod :: any).eval(source, env)
end

local function apply_binding_mapping(spec: any, input: Map): (any?, any)
    local mapping = type(spec) == "table" and (spec :: Map) or {}
    if next(mapping) == nil then return input, nil end
    local mode = trim(mapping.mode)
    if mode == "" then mode = "expr" end
    if mode ~= "expr" and mode ~= "expr_generated" then return nil, "binding mapping mode unsupported: " .. mode end
    local source = trim(mapping.expr)
    if source == "" then return nil, "binding mapping expr is required" end
    local out, err = eval_expr(source, { input = input })
    if err then return nil, "binding mapping expr: " .. tostring(err) end
    return out, nil
end

local function binding_guard_allows(binding: Binding, input: Map): (boolean, any)
    local guard = trim(binding.guard_expr)
    if guard == "" then return true, nil end
    local out, err = eval_expr(guard, { input = input })
    if err then return false, "binding guard expr: " .. tostring(err) end
    return not (out == nil or out == false), nil
end

M._revalidate_binding_dispatch = function(binding: any, _context: any): (any?, any)
    local b = type(binding) == "table" and (binding :: Map) or {}
    local component = dependencies.component :: automation_types.ComponentModule

    local policy = type(b.execution_policy) == "table" and (b.execution_policy :: Map) or {}
    local mode = trim(policy.mode)
    if mode == "" then mode = "component_owner" end

    local subject_id = ""
    if mode ~= "component_owner" then
        return nil, "unsupported binding execution_policy.mode: " .. mode
    end
    local owner, owner_err = (component :: any).component_owner(trim(b.binding_id))
    if owner_err then return nil, "binding component owner lookup failed: " .. tostring(owner_err) end
    subject_id = trim(owner)
    if subject_id == "" then return nil, "binding component owner unavailable: " .. trim(b.binding_id) end

    local ctx, ctx_err = (component :: any).get_private_context(trim(b.binding_id))
    if ctx_err then return nil, "binding execution identity lookup failed: " .. tostring(ctx_err) end
    if type(ctx) ~= "table" then return nil, "binding execution identity unavailable: " .. trim(b.binding_id) end
    local identity = (ctx :: Map)[EXECUTION_IDENTITY_KEY]
    if type(identity) ~= "table" then return nil, "binding execution identity unavailable: " .. trim(b.binding_id) end

    local app_scope: string? = nil
    if type(b.authority_scope) == "table" and (b.authority_scope :: Map).app_scope ~= nil then
        local configured_scope = (b.authority_scope :: Map).app_scope
        if type(configured_scope) ~= "string" or trim(configured_scope) == "" then
            return nil, "binding authority_scope.app_scope is invalid"
        end
        app_scope = configured_scope
    end
    local actor, scope, ident_err = execution_identity.reconstruct_row(identity.actor_id, identity.actor_context, {
        live = true,
        subject_id = subject_id,
        app_scope = app_scope,
    })
    if ident_err or not actor or not scope then
        return nil, "binding dispatch identity revalidation failed: " .. tostring(ident_err or "no actor/scope")
    end

    return {
        success = true,
        actor = actor,
        scope = scope,
        authority_identity = {
            mode = mode,
            actor_id = subject_id,
            installed_by = trim(b.created_by) ~= "" and trim(b.created_by) or nil,
        },
    }, nil
end

M._launch_flow = function(args: any, authority: any?): (any?, any)
    local a = type(args) == "table" and (args :: Map) or {}
    local flow_ref, ref_err = exact_flow_ref(a.flow_ref)
    if ref_err or not flow_ref then return nil, ref_err end
    local runtime, runtime_err = M._open_flow_runtime(authority)
    if runtime_err or not runtime then return nil, runtime_err end
    return runtime:execute({
        flow_ref = flow_ref,
        input = type(a.input) == "table" and a.input or {},
        opts = {
            mode = a.mode,
            dedup_key = a.dedup_key,
            trace_context = a.trace_context,
            authority_identity = a.authority_identity,
            trigger_kind = a.trigger_kind,
            trigger_id = a.trigger_id,
            automation_ref = a.automation_ref,
            invocation_id = a.invocation_id,
        },
    })
end

-- A due cron row can already be claimed while DELETE (or pause) synchronously
-- reaps the binding-owned lowering. The binding snapshot used to map the fire is
-- therefore not enough authority to launch a run: re-read the small binding row
-- at the launch boundary for timer fires.
local function periodic_fire_dispatchable(binding_id: string, expected_revision: number, claimed_schedule_id: string): (boolean?, string?, any)
    local current, read_err = get_binding_row(binding_id)
    if read_err then return nil, nil, read_err end
    if not current then return false, "binding_missing", nil end
    if current.enabled ~= true then return false, "disabled", nil end
    if current.revision ~= expected_revision then return false, "binding_replaced", nil end
    local lowering = type(current.lowering_state) == "table" and (current.lowering_state :: Map) or {}
    local registration = type(lowering.registration) == "table" and (lowering.registration :: Map) or {}
    if claimed_schedule_id == "" or trim(registration.schedule_id) ~= claimed_schedule_id then
        return false, "stale_schedule", nil
    end
    return true, nil, nil
end

local function skip_periodic_fire(binding_id: string, reason: string): Map
    local logger_mod = dependencies.logger :: LoggerModule
    logger_mod:named("automations.binding_dispatch"):debug(
        "periodic binding is no longer dispatchable; dropping claimed fire", {
            binding_id = binding_id,
            reason = reason,
        })
    -- This is a terminal skip for the already-claimed fire. Returning success
    -- prevents the scheduler from retrying a binding its owner has removed (or
    -- paused); DELETE has already removed the recurring cron row itself.
    return { success = true, skipped = true, terminal = true, reason = reason }
end

-- One run of one binding. The delivery boundary below wraps it; nothing else
-- calls it, so a batch reports exactly one runtime outcome instead of one per
-- item.
local execute_run: (string, Map) -> (Map?, any)

local function execute_envelope_batch(binding_id: string, envelopes: { any }): (Map?, any)
    local failed_keys: { string } = {}
    -- Keyed delivery failures are terminal for this cursor item: the
    -- projection may advance, but its durable last_error must name what was
    -- skipped. Keep the association here while the envelope still has both
    -- the idempotency key and the launch error.
    local failure_details: { Map } = {}
    local delivered = 0
    for _, envelope in ipairs(envelopes) do
        local env = type(envelope) == "table" and (envelope :: Map) or {}
        local res, err = execute_run(binding_id, {
            binding_id = binding_id,
            context = env.item,
            occurred_at = env.occurred_at,
            event_type = env.event_type,
            dedup_key = env.dedup_key,
            trace_context = env.trace_context,
        })
        if err or (type(res) == "table" and (res :: Map).success == false) then
            local key = trim(env.dedup_key)
            if key == "" then return nil, err or tostring((res :: Map).error or "binding execution failed") end
            failed_keys[#failed_keys + 1] = key
            failure_details[#failure_details + 1] = {
                dedup_key = key,
                error = tostring(err or (type(res) == "table" and (res :: Map).error) or "binding execution failed"),
            }
        else
            delivered = delivered + 1
        end
    end
    return {
        success = true,
        delivered = delivered,
        skipped = 0,
        failed_keys = failed_keys,
        failure_details = failure_details,
    }, nil
end

-- last_error is the binding's own delivery outcome, so the binding publishes it
-- as each run terminates: named on failure, cleared on success. A skipped fire
-- (paused, replaced, guarded away) delivered nothing and says nothing about the
-- machine, so it leaves the published outcome standing.
local function publish_delivery_outcome(binding_id: string, result: Map?, err: any): ()
    if err ~= nil then
        publish_binding_runtime(binding_id, { last_error = tostring(err) })
        return
    end
    local r = type(result) == "table" and (result :: Map) or {}
    if r.skipped == true then return end
    local details = type(r.failure_details) == "table" and (r.failure_details :: { any }) or {}
    local keys = type(r.failed_keys) == "table" and (r.failed_keys :: { any }) or {}
    local last_error = ""
    if #details > 0 then
        local first = type(details[1]) == "table" and (details[1] :: Map) or {}
        last_error = "delivery failed for " .. tostring(#keys) .. " item(s): "
            .. tostring(first.error or "binding execution failed")
    elseif #keys > 0 then
        last_error = "delivery failed for " .. tostring(#keys) .. " item(s)"
    end
    local runtime_patch: Map = { last_error = last_error }
    -- Poll delivery returns an exact page acknowledgement count. Publish it
    -- alongside the frozen item budget so the generic Automation read model
    -- exposes what the runtime actually accepted; no provider/config metadata
    -- is used as a substitute for this runtime count.
    if r.delivered ~= nil then
        local delivered = math.max(0, math.floor(tonumber(r.delivered) or 0))
        runtime_patch.accepted_count = delivered
        runtime_patch.last_run_accepted = delivered
    end
    publish_binding_runtime(binding_id, runtime_patch)
end

function M.execute_binding(args: any): (Map?, any)
    local a: Map = type(args) == "table" and (args :: Map) or {}
    local binding_id = trim(a.binding_id)
    if binding_id == "" then return nil, "binding_id is required" end
    if a.item ~= nil then return nil, "binding execution input must use context" end
    if a.context ~= nil and type(a.context) ~= "table" then return nil, "context must be an object" end

    local result: Map?
    local err: any
    if type(a.envelopes) == "table" then
        result, err = execute_envelope_batch(binding_id, a.envelopes :: { any })
    else
        result, err = execute_run(binding_id, a :: any)
    end
    publish_delivery_outcome(binding_id, result, err)
    return result, err
end

execute_run = function(binding_id: string, a: Map): (Map?, any)
    local binding, berr = get_binding_row(binding_id)
    if berr then return nil, berr end
    local periodic_fire = a.event_type == "schedule"
    if not binding then
        if periodic_fire then return skip_periodic_fire(binding_id, "binding_missing"), nil end
        return nil, "binding not found: " .. binding_id
    end
    if binding.enabled ~= true then return { success = true, skipped = true, reason = "disabled" }, nil end

    local binding_ref, ref_err = automation_ref.validate({
        kind = "automation",
        id = binding.binding_id,
        revision = binding.revision,
    })
    if ref_err or not binding_ref then return nil, ref_err or "invalid automation_ref" end

    local raw_context = type(a.context) == "table" and a.context or {}
    local input = mapping_input_context(binding, raw_context, a, binding_ref :: Map)
    local keep, gerr = binding_guard_allows(binding, input)
    if gerr then return nil, gerr end
    if not keep then return { success = true, skipped = true, reason = "guard" }, nil end
    local mapped, merr = apply_binding_mapping(binding.mapping_spec, input)
    if merr then return nil, merr end

    local authority, aerr = M._revalidate_binding_dispatch(binding, input)
    if aerr or not authority then return nil, aerr or "binding dispatch revalidation failed" end
    local auth = type(authority) == "table" and (authority :: Map) or {}

    local trace_context = type(a.trace_context) == "table" and a.trace_context
        or (type(input.trace_context) == "table" and input.trace_context or nil)
    local policy = type(binding.execution_policy) == "table" and (binding.execution_policy :: Map) or {}
    local launch_args: Map = {
        flow_ref = binding.flow_ref,
        input = type(mapped) == "table" and mapped or { value = mapped },
        mode = trim(policy.launch_mode) ~= "" and policy.launch_mode or "async",
        dedup_key = trim(a.dedup_key) ~= "" and trim(a.dedup_key) or nil,
        trace_context = trace_context,
        authority_identity = auth.authority_identity,
        trigger_kind = "binding",
        trigger_id = trigger_catalog_id(binding.trigger),
        automation_ref = copy_map(binding_ref),
        invocation_id = trim(a.invocation_id) ~= "" and trim(a.invocation_id) or nil,
    }
    if periodic_fire then
        local claimed_schedule_id = trim((raw_context :: Map).schedule_id)
        local dispatchable, reason, fire_err = periodic_fire_dispatchable(
            binding.binding_id,
            binding.revision,
            claimed_schedule_id
        )
        if fire_err then return nil, fire_err end
        if dispatchable ~= true then
            return skip_periodic_fire(binding.binding_id, reason or "binding_missing"), nil
        end
    end
    local out, lerr = M._launch_flow(launch_args, auth)
    if lerr then return nil, lerr end
    local result = type(out) == "table" and (out :: Map) or {}
    result.success = result.success ~= false
    return result, nil
end

function M.execute_binding_action(args: any): (Map?, any)
    local a = type(args) == "table" and (args :: Map) or {}
    local id = trim(a.binding_id)
    if id == "" then
        local ctx_mod = dependencies.ctx
        if ctx_mod then id = trim((ctx_mod :: any).get("component_id")) end
    end
    local context = a.context
    local fire_timestamp: string? = nil
    if context == nil and type(a._schedule) == "table" then
        local sched = a._schedule :: Map
        fire_timestamp = trim(sched.fired_at)
        if fire_timestamp == "" then fire_timestamp = now_rfc3339() end
        local schedule_context = type(sched.schedule) == "table" and copy_map(sched.schedule) or {}
        schedule_context.schedule_id = schedule_context.schedule_id or sched.schedule_id
        schedule_context.name = schedule_context.name or sched.name
        context = {
            schedule_id = sched.schedule_id,
            previous_runs = sched.previous_runs,
            fired_at = fire_timestamp,
            fire_timestamp = fire_timestamp,
            occurred_at = fire_timestamp,
            schedule = schedule_context,
        }
    end
    local schedule_dedup: string? = nil
    if type(a._schedule) == "table" then
        local sched = a._schedule :: Map
        local schedule_id = trim(sched.schedule_id)
        local fired_at = fire_timestamp or trim(sched.fired_at)
        if schedule_id ~= "" and fired_at ~= "" then schedule_dedup = schedule_id .. ":" .. fired_at end
    end
    local result, execute_err = M.execute_binding({
        binding_id = id,
        context = context or {},
        occurred_at = type(context) == "table" and (context :: Map).occurred_at or nil,
        event_type = type(a._schedule) == "table" and "schedule" or nil,
        dedup_key = a.dedup_key or schedule_dedup,
        trace_context = a.trace_context,
    })
    if execute_err or type(result) ~= "table" then return nil, execute_err or "binding execution failed" end
    return result :: Map, nil
end

function M.deliver_binding(args: any): (Map?, any)
    local a = type(args) == "table" and (args :: Map) or {}
    if type(a.envelopes) ~= "table" then return nil, "envelopes must be an array" end
    local ctx_mod = dependencies.ctx
    local binding_id = ctx_mod and trim((ctx_mod :: any).get("component_id")) or ""
    if binding_id == "" then return nil, "component_id not in scope" end
    local result, execute_err = M.execute_binding({ binding_id = binding_id, envelopes = a.envelopes })
    if execute_err or type(result) ~= "table" then return nil, execute_err or "binding execution failed" end
    return result :: Map, nil
end

-- resolve_io_port recovers a port descriptor by its PORT ENTRY id — the
-- descriptor the run path (write_to_sink / read_from_source) needs to act through
-- the port's backing. Stored specs carry port entry ids (v3 cutover), so the
-- match is by `id` only. Returns nil with an error when no port declares the id.
local function resolve_io_port(key: string, dir: string, label: string): (Map?, string?)
    if type(key) ~= "string" or key == "" then return nil, label .. " port is required" end
    -- Point resolution is a map hit on the snapshot the build just held or
    -- published; only listings walk and sort the catalog.
    local _, err = build_catalog()
    if err then return nil, tostring(err) end
    local by_id = catalog_snapshot.by_id
    if type(by_id) == "table" then
        -- The snapshot is authoritative for its registry version: absent or
        -- wrong-direction means not found, never a fallback to older state.
        local desc = (by_id :: { [string]: Map })[key]
        if desc ~= nil and surface_is_dir(tostring((desc :: Map).surface or ""), dir) then
            return desc, nil
        end
        return nil, label .. " port not found: " .. key
    end
    -- No snapshot capability on this runtime: walk the freshly built list.
    local list, lerr = list_io(dir)
    if lerr then return nil, tostring(lerr) end
    for _, raw in ipairs(list or {}) do
        local desc = raw :: Map
        if tostring(desc.id or "") == key then
            return (desc :: Map?), nil
        end
    end
    return nil, label .. " port not found: " .. key
end

-- resolve_sink / resolve_source: the port descriptor an automation stored
-- under its sink / source port id, recovered at run time.
function M.resolve_sink(port_id: string): (Map?, string?)
    local desc, err = resolve_io_port(port_id, "in", "sink")
    return (desc :: Map?), err
end

function M.resolve_source(port_id: string): (Map?, string?)
    local desc, err = resolve_io_port(port_id, "out", "source")
    return (desc :: Map?), err
end

-- The deterministic per-actor component id for a class. A per-user singleton
-- source component (uploads, inbox) has exactly one instance, so a trigger on it
-- resolves the instance from the installing actor + class instead of prompting a
-- picker that would only ever offer one choice.
function M.component_id_for_actor(class: string): (string?, string?)
    if type(class) ~= "string" or class == "" then return nil, "source declares no class" end
    local sec = dependencies.security
    local actor = sec and (sec :: any).actor()
    if not actor then return nil, "authentication required" end
    local actor_id = (actor :: any):id()
    if type(actor_id) ~= "string" or actor_id == "" then return nil, "actor id unavailable" end
    local id = (dependencies.autoinit :: any).component_id(actor_id, class)
    return (id :: string), nil
end

local WRITABLE_CONTRACT = "kickside.data:writable"

-- Writable ports select component-backed destinations through their picker
-- configuration.  The canonical picker spelling is thread_id because a
-- component's primary thread is its stable public handle; contract bindings,
-- however, run component methods under ctx.component_id.  Preserve every
-- picker value while carrying that selected component identity into the open
-- context.  An explicit component_id always wins for ports that already use
-- the canonical spelling directly.
local function sink_open_context(config: Map): Map
    local context = copy_map(config) :: any
    local component_id = trim((context :: any).component_id)
    if component_id == "" then component_id = trim((context :: any).thread_id) end
    if component_id ~= "" then (context :: any).component_id = component_id end
    -- Sink writes run under the automation runtime. Declaring it in the open
    -- context lets destination write paths distinguish machine-asserted
    -- provenance from an ambient human session instead of stripping it.
    if trim((context :: any).runtime_type) == "" then (context :: any).runtime_type = "automation" end
    return context :: Map
end

function M.open_sink_writer(binding: string, config: Map): (any?, string?)
    if type(binding) ~= "string" or binding == "" then return nil, "sink binding is required" end
    local contract_mod = dependencies.contract
    local def, derr = contract_mod.get(WRITABLE_CONTRACT)
    if derr or not def then return nil, "writable unavailable: " .. tostring(derr) end
    local context = sink_open_context(config or {})
    local inst, oerr = (def :: any):with_context(context):open(binding)
    if oerr or not inst then return nil, "writable open: " .. tostring(oerr) end
    return {
        -- A failed write answers (nil, message, envelope): the message is the
        -- keyed failure shape every dispatcher records, the envelope is the
        -- sink's own structured error ({code, retriable, dependency_key, ...})
        -- for callers that classify failures instead of reading text.
        write = function(_self: any, body: Map): (any?, string?, Map?)
            local res, werr = (inst :: any):write(body)
            if werr then return nil, tostring(werr) end
            if type(res) ~= "table" then
                return nil, "sink write returned a non-object result"
            end
            local result = res :: Map
            if result.success ~= true then
                local rerr: any = result.error
                local emsg = (type(rerr) == "table" and (rerr.message or rerr.code)) or rerr
                    or "sink write did not acknowledge success=true"
                local envelope: Map? = nil
                if type(rerr) == "table" then envelope = rerr :: Map end
                return nil, tostring(emsg), envelope
            end
            return result, nil
        end,
    }, nil
end

-- dispatch_to_sink opens the kickside.data:writable backing of `binding` with the
-- sink `config` (the picker values) as the open context, so a binding declaring
-- context_required reads them, and writes `body` (the writable ABI envelope the
-- caller builds: config + sink_op + input + idempotency_key). A write must explicitly
-- acknowledge success=true; any other result is surfaced as an error so every dispatcher
-- records and retries through one path. Returns the writable result, or an error.
function M.dispatch_to_sink(binding: string, config: Map, body: Map): (any?, string?)
    local writer, err = M.open_sink_writer(binding, config)
    if err or not writer then return nil, err end
    return (writer :: any):write(body)
end


-- list_installed: read umbrella components from userspace, project state,
-- merge reportable.status. opts: { actor_id? = string, class? = string,
-- limit? = number, offset? = number }.
local function flow_ref_for_list_row(meta: Map): Map?
    if type(meta.flow_ref) == "table" then return decode_json_table(meta.flow_ref) end
    return nil
end

-- Bindings hold an exact, unrepairable reference to their destination, so a
-- removed destination takes its bindings with it. Resource identity here is the
-- destination reference itself, which means no provider declares a second
-- vocabulary to be found by.
local function bindings_for_destination(resource_kind: string, resource_id: string): ({ string }?, any)
    local db, db_err = binding_db()
    if not db then return nil, db_err end
    local rows, query_err = db:query(
        "SELECT binding_id, flow_ref FROM " .. AUTOMATION_BINDING_TABLE .. " ORDER BY binding_id",
        {})
    db:release()
    if query_err then return nil, query_err end
    local out: { string } = {}
    for _, row in ipairs(rows or {}) do
        local ref = decode_json_table((row :: Map).flow_ref)
        if trim(ref.kind) == resource_kind and trim(ref.id) == resource_id then
            out[#out + 1] = tostring((row :: Map).binding_id)
        end
    end
    return out, nil
end

-- A trigger machine's drain phase in the vocabulary the control surface reads.
local PHASE_STATUS: { [string]: string } = {
    paused = "paused",
    backfilling = "running",
    failed = "failed",
    invalid = "failed",
    live = "idle",
}

local function status_for_phase(enabled: boolean, phase: any): string
    if not enabled then return "paused" end
    return PHASE_STATUS[trim(phase)] or "idle"
end

-- Cadence is a property of the stored spec, not of the running machine: a timer
-- binding carries its schedule, a source binding fires on its source's events.
local function binding_cadence(trigger: any): (string, string)
    local spec = type(trigger) == "table" and (trigger :: Map) or {}
    local schedule = type(spec.schedule) == "table" and (spec.schedule :: Map) or nil
    if schedule then return trim(schedule.type), trim(schedule.expression) end
    if trim(spec.source) ~= "" then return "event", "" end
    return "", ""
end

-- Binding enabled/status is owned by the binding row, not duplicated into
-- component metadata. Project the authoritative rows once per list request.
--
-- Listing acquires nothing. Every contract method is a `function.lua` invocation
-- that instantiates its own import graph, and this is the most repeated read in
-- the product, so a per-row contract call multiplies without bound. The row's own
-- columns already carry enabled, cadence, and the recorded drain phase; live
-- machine state belongs to the single-row surfaces a person opens by hand.
local function binding_state_rows(components: { automation_types.ComponentRow }): ({ [string]: BindingStatus }?, any)
    local ids: { string } = {}
    for _, c in ipairs(components) do
        if c.impl_id == AUTOMATION_BINDING_KIND then ids[#ids + 1] = tostring(c.component_id) end
    end
    if #ids == 0 then return {}, nil end
    local db, db_err = binding_db()
    if not db then return nil, db_err end
    -- Bounded by the rows on this page, never the whole table.
    local placeholders: { string } = {}
    for index, _ in ipairs(ids) do placeholders[index] = "$" .. tostring(index) end
    local rows, query_err = db:query(
        "SELECT binding_id, enabled, trigger_config, lowering_state FROM " .. AUTOMATION_BINDING_TABLE
            .. " WHERE binding_id IN (" .. table.concat(placeholders, ", ") .. ")",
        ids)
    db:release()
    if query_err then return nil, query_err end
    local out: { [string]: BindingStatus } = {}
    for _, row in ipairs(rows or {}) do
        local r = row :: Map
        local enabled = is_enabled_value(r.enabled)
        local lowering = decode_json_table(r.lowering_state)
        local trigger = decode_json_table(r.trigger_config)
        local schedule_type, schedule_expression = binding_cadence(trigger)
        local state: BindingStatus = {
            enabled = enabled,
            status = status_for_phase(enabled, lowering.phase),
            schedule_type = schedule_type,
            schedule_expression = schedule_expression,
        }
        local effective_spec = type(lowering.spec) == "table" and (lowering.spec :: Map) or trigger
        if tostring(lowering.mode or "") == "poll" and type(effective_spec.poll) == "table" then
            local poll = effective_spec.poll :: Map
            state.max_pull_pages_per_run = poll.max_pages_per_run
            state.max_items_per_run = poll.max_items_per_run
        end
        out[tostring(r.binding_id)] = state
    end
    return out, nil
end

function M.list_installed(opts: automation_types.ListInstalledOptions?): ({automation_types.InstalledAutomation}?, any)
    opts = opts or {}
    local limit, limit_err = pagination_int(opts.limit, "limit")
    if limit_err then return nil, limit_err end
    local offset, offset_err = pagination_int(opts.offset, "offset")
    if offset_err then return nil, offset_err end

    local component = dependencies.component :: automation_types.ComponentModule
    local registry = dependencies.registry :: automation_types.RegistryModule

    -- Read every umbrella by binding impl id (any class), then narrow with
    -- class_matches below. Filtering the query by meta.class would drop
    -- umbrellas whose class isn't the literal "automation" (e.g. knowledge).
    local impl_ids = automation_binding_ids()
    if #impl_ids == 0 then return {}, nil end

    -- "Automations that run this destination" is destination identity, not
    -- placement: a binding addressed at a destination belongs to that answer
    -- wherever in the workspace it is filed. Placement stays a separate filter.
    local destination_id = trim(opts.destination_id)
    local destination_ids: { string }? = nil
    if destination_id ~= "" then
        local kind = trim(opts.destination_kind)
        local matched, match_err = bindings_for_destination(kind ~= "" and kind or "flow", destination_id)
        if match_err then return nil, match_err end
        if not matched or #matched == 0 then return {}, nil end
        destination_ids = matched
    end
    local parent_id = type(opts.parent_id) == "string" and opts.parent_id ~= "" and opts.parent_id or nil

    -- Actor-scoped read goes through the access-filtered read-port; the no-actor
    -- path is a trusted system listing (e.g. background reconciliation).
    local components: { automation_types.ComponentRow }
    if type(opts.actor_id) == "string" and opts.actor_id ~= "" then
        components = (component.query({
            actor_id = opts.actor_id,
            impl_ids = impl_ids,
            component_ids = destination_ids,
            parent_id = destination_ids == nil and parent_id or nil,
            include = { meta = true, access = true, placement = true },
            order_by = { field = "created_at", direction = "DESC" },
            limit = limit,
            offset = offset,
        }) or {}) :: { automation_types.ComponentRow }
    else
        components = (component.list_system({
            impl_ids = impl_ids,
            component_ids = destination_ids,
            parent_id = destination_ids == nil and parent_id or nil,
            include = { meta = true, placement = true },
            order_by = { field = "created_at", direction = "DESC" },
            limit = limit,
            offset = offset,
        }) or {}) :: { automation_types.ComponentRow }
    end
    local binding_state_ok, binding_state, binding_state_err = pcall(binding_state_rows, components)
    if not binding_state_ok then return nil, "binding_state_rows panic: " .. tostring(binding_state) end
    if binding_state_err then return nil, binding_state_err end
    -- Per-impl_id public_state_schema cache: every component of one type shares
    -- the same schema, so it is resolved once, not per row. A cache value of
    -- false marks a type that declares no schema (distinct from "not yet looked
    -- up") so that too is memoized. The materialized public_state already lives
    -- in c.meta (include = meta above) -- projecting it is pure, no extra query.
    local schema_cache: { [string]: any } = {}
    local out: {automation_types.InstalledAutomation} = {}
    for _, c in ipairs(components) do
        local meta_t: Map = c.meta or {}
        if M.class_matches(meta_t.class, opts.class) then
            local entry: automation_types.InstalledAutomation = {
                id           = c.component_id,
                type         = c.impl_id,
                title        = meta_t.title or "",
                icon         = meta_t.icon or "",
                description  = meta_t.comment or "",
                class        = meta_t.class :: automation_types.ClassValue?,
                created_at   = c.created_at,
                updated_at   = c.updated_at,
                access_level = c.access_level or 0,
                parent_id    = c.parent_id,
            }
            local flow_ref = flow_ref_for_list_row(meta_t)
            if flow_ref then entry.flow_ref = flow_ref end
            local impl_id = c.impl_id
            if type(impl_id) == "string" and impl_id ~= "" then
                local cached = schema_cache[impl_id]
                if cached == nil then
                    local schema = public_state_schema_for_impl(registry, impl_id)
                    cached = schema or false
                    schema_cache[impl_id] = cached
                end
                if cached ~= false then
                    local state_meta = meta_t
                    if impl_id == AUTOMATION_BINDING_KIND and binding_state and binding_state[tostring(c.component_id)] then
                        state_meta = copy_map(meta_t)
                        for key, value in pairs(binding_state[tostring(c.component_id)]) do state_meta[key] = value end
                    end
                    local projection_ok, projected, projection_err = pcall(project_public_state, cached, state_meta)
                    if not projection_ok then return nil, "public state projection panic: " .. tostring(projected) end
                    if projection_err then return nil, projection_err end
                    if projected then entry.public_state = projected :: Map end
                end
            end
            out[#out + 1] = entry
        end
    end
    return out, nil
end

-- M.list_runtime enumerates installed automations with their engine-owned
-- execution identity, for event-driven runtimes (gateway hubs/workers) that must
-- run an automation as its installer. opts:
--   { type_ids?, type_meta?, class?, limit?, offset?, include_state?, actor_id? }
-- type_ids selects exact binding ids; type_meta selects them by binding meta
-- (e.g. { provider = "discord" }); neither => all automation types.
-- actor_id scopes the read through the access filter; omit for a trusted system
-- read (a gateway hub). This is THE authority a hub asks -- not component meta
-- queries, not raw private_context reads.
function M.list_runtime(opts: any): ({automation_types.RuntimeAutomation}?, any)
    opts = type(opts) == "table" and opts or {}
    local component = dependencies.component :: automation_types.ComponentModule

    local impl_ids: {string}
    if type(opts.type_ids) == "table" and #(opts.type_ids :: {string}) > 0 then
        impl_ids = opts.type_ids :: {string}
    elseif type(opts.type_meta) == "table" then
        impl_ids = binding_ids_by_meta(opts.type_meta)
    else
        impl_ids = automation_binding_ids()
    end
    if #impl_ids == 0 then return {}, nil end

    local limit, limit_err = pagination_int(opts.limit, "limit")
    if limit_err then return nil, limit_err end
    local offset, offset_err = pagination_int(opts.offset, "offset")
    if offset_err then return nil, offset_err end

    local rows: { automation_types.ComponentRow }
    if type(opts.actor_id) == "string" and opts.actor_id ~= "" then
        rows = (component.query({
            actor_id = opts.actor_id, impl_ids = impl_ids, include = { meta = true },
            order_by = { field = "created_at", direction = "DESC" }, limit = limit, offset = offset,
        }) or {}) :: { automation_types.ComponentRow }
    else
        rows = (component.list_system({
            impl_ids = impl_ids, include = { meta = true },
            order_by = { field = "created_at", direction = "DESC" }, limit = limit, offset = offset,
        }) or {}) :: { automation_types.ComponentRow }
    end

    local include_state = opts.include_state == true
    local out: {automation_types.RuntimeAutomation} = {}
    for _, c in ipairs(rows) do
        local meta_t: Map = c.meta or {}
        if M.class_matches(meta_t.class, opts.class) then
            local ctx = component.get_private_context(c.component_id)
            local identity: any = nil
            local state: any = nil
            if type(ctx) == "table" then
                identity = (ctx :: Map)[EXECUTION_IDENTITY_KEY]
                if include_state then state = strip_reserved_state(ctx) end
            end
            out[#out + 1] = ({
                id = c.component_id,
                type = c.impl_id,
                metadata = meta_t,
                execution_identity = identity,
                state = state,
            } :: automation_types.RuntimeAutomation)
        end
    end
    return out, nil
end

-- M.get_runtime returns one installed automation by component id with its type,
-- execution identity, and optional state.
function M.get_runtime(opts: any): (automation_types.RuntimeAutomation?, any)
    opts = type(opts) == "table" and opts or {}
    local id = opts.id
    if type(id) ~= "string" or id == "" then return nil, "id is required" end
    local component = dependencies.component :: automation_types.ComponentModule
    local rows = (component.list_system({
        component_ids = { id :: string },
        include = { meta = true },
    }) or {}) :: { automation_types.ComponentRow }
    local row = rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then return nil, "automation not found" end
    local ctx, ctx_err = component.get_private_context(id :: string)
    if ctx_err or type(ctx) ~= "table" then return nil, ctx_err or "automation not found" end
    local state: any = nil
    if opts.include_state == true then state = strip_reserved_state(ctx) end
    return ({
        id = id :: string,
        type = row.impl_id,
        metadata = row.meta or {},
        execution_identity = (ctx :: Map)[EXECUTION_IDENTITY_KEY],
        state = state,
    } :: automation_types.RuntimeAutomation), nil
end

-- read_public_state is the frontend-safe public state helper.
-- The storage plane is component public meta: schema fields are normal meta keys
-- so list/render/realtime all read one public component surface. Scheduler state
-- stays behind cron; private runtime state stays in private_context.
function M.read_public_state(component_id: string): (Map?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local component = dependencies.component :: automation_types.ComponentModule
    local registry = dependencies.registry :: automation_types.RegistryModule
    local binding_rows, binding_query_err = component.query({
        component_ids = { component_id },
        include = { meta = true },
        access_mask = component.ACCESS.READ,
    })
    if binding_query_err then return nil, binding_query_err end
    local binding_row = binding_rows and binding_rows[1]
    if binding_row and binding_row.impl_id == AUTOMATION_BINDING_KIND then
        local binding_raw, binding_err = M.get_binding(component_id)
        if binding_err or type(binding_raw) ~= "table" then return nil, binding_err or "automation not found" end
        return binding_public_state(binding_raw :: Binding), nil
    end
    local schema, row, schema_err = public_state_component(component, registry, component_id, component.ACCESS.READ)
    if schema_err then return nil, schema_err end
    local projected, project_err = project_public_state(schema, row and row.meta or {})
    if project_err then return nil, project_err end
    local out: Map
    if projected then
        out = (projected :: Map)
    else
        out = {}
    end
    return (out :: Map), nil
end

-- write_public_state partially updates a component's public state (its render/read
-- model) through component public meta, validating each provided field against the
-- type's public_state_schema. WRITE-gated. Automation control methods (pause/resume,
-- status transitions) use it so the card and list reflect a state change live via the
-- component.meta.changed relay, without owning a projection.
function M.write_public_state(component_id: string, patch: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(patch) ~= "table" then return nil, "patch must be a table" end
    local component = dependencies.component :: automation_types.ComponentModule
    local registry = dependencies.registry :: automation_types.RegistryModule
    local schema, _, schema_err = public_state_component(component, registry, component_id, component.ACCESS.WRITE)
    if schema_err then return nil, schema_err end
    local allowed_map, fields_err = public_state_field_map(schema)
    if fields_err then return nil, fields_err end
    local allowed: { [string]: any } = allowed_map or {}

    -- title/comment are core component-meta fields (not schema-declared read-model
    -- fields); a reconfigure that renames writes them alongside its public_state.
    local CORE_META: { [string]: boolean } = { title = true, comment = true }
    local fields: Map = {}
    for k, v in pairs(patch :: Map) do
        if type(k) ~= "string" then
            return nil, "public_state field keys must be strings"
        elseif CORE_META[k] then
            fields[k] = tostring(v)
        elseif allowed[k] ~= nil then
            local verr = validate_public_field_value(allowed[k], v, patch)
            if verr then return nil, verr end
            fields[k] = v
        else
            return nil, "public_state contains undeclared field: " .. tostring(k)
        end
    end
    if next(fields) == nil then return nil, "patch must set at least one field" end

    local ok, set_err = (component :: any).set_meta(component_id, fields)
    if not ok then return nil, set_err end
    return { success = true, id = component_id, public_state = fields }, nil
end

-- read_state / patch_state are runtime helpers for automation code that owns
-- private durable state. Frontend APIs should use the public-state helpers.
function M.read_state(component_id: string, required_access: integer?): (automation_types.AutomationPrivateContext?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local component = dependencies.component :: automation_types.ComponentModule
    local access = required_access or component.ACCESS.READ
    local ctx, err = component.get_context(component_id, access)
    if err then return nil, err end
    return copy_private_state(ctx, true), nil
end

-- read_trigger_state is the engine-only reader paired with the reserved trigger
-- writers below. Product automations use read_state and never receive these
-- lifecycle/progress blocks.
function M.read_trigger_state(component_id: string, required_access: integer?): (Map?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local component = dependencies.component :: automation_types.ComponentModule
    local access = required_access or component.ACCESS.READ
    local ctx, err = component.get_context(component_id, access)
    if err then return nil, err end
    local raw = type(ctx) == "table" and (ctx :: Map) or {}
    return {
        [TRIGGER_STATE_KEY] = raw[TRIGGER_STATE_KEY],
        [TRIGGER_PROGRESS_KEY] = raw[TRIGGER_PROGRESS_KEY],
    }, nil
end

-- Mutating trigger lifecycle operations authorize before touching their mode.
-- Keep the access mask inside the engine that owns component integration.
function M.read_trigger_state_for_update(component_id: string): (Map?, any)
    local component = dependencies.component :: automation_types.ComponentModule
    local state, err = M.read_trigger_state(component_id, component.ACCESS.WRITE)
    return state, err
end

-- consumer_delivery_options reads one automation type's declared delivery
-- options off its binding meta. deliver_empty opts the type into receiving a
-- batch that carries no envelopes: the dispatch seam otherwise short-circuits an
-- empty batch, which is right for a consumer whose only work is the items and
-- wrong for one that owes end-of-round work (the sync's armed superseded sweep)
-- the round would otherwise never reach. Types that declare nothing keep the
-- short-circuit, so this never changes an existing consumer's delivery shape.
function M.consumer_delivery_options(impl_id: string): (Map, any)
    local defaults: Map = { deliver_empty = false }
    if type(impl_id) ~= "string" or impl_id == "" then return defaults, nil end
    local registry = dependencies.registry :: automation_types.RegistryModule
    local entry, get_err = registry.get(impl_id)
    if get_err then return defaults, get_err end
    if type(entry) ~= "table" or entry.kind ~= "contract.binding" then return defaults, nil end
    local meta: Map = entry.meta or {}
    return { deliver_empty = meta.deliver_empty == true }, nil
end

-- read_trigger_liveness answers "is this trigger still running?" from the
-- engine-owned blocks alone: the poll machine stamps last_tick_at on every
-- committed round, so a caller distinguishes a live idle trigger from a stopped
-- one without a delivery having happened and without opening the scheduler.
-- READ-gated through the same component context every other reader uses.
function M.read_trigger_liveness(component_id: string): (Map?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local component = dependencies.component :: automation_types.ComponentModule
    local ctx, err = component.get_context(component_id, component.ACCESS.READ)
    if err then return nil, err end
    local raw = type(ctx) == "table" and (ctx :: Map) or {}
    local block = raw[TRIGGER_STATE_KEY]
    if type(block) ~= "table" then return nil, nil end
    local b = block :: Map
    local progress = type(raw[TRIGGER_PROGRESS_KEY]) == "table" and (raw[TRIGGER_PROGRESS_KEY] :: Map) or {}
    local out: Map = {
        mode = b.mode,
        phase = b.phase == "paused" and "paused" or (progress.phase or b.phase),
        revision = tonumber(b.revision) or 1,
    }
    if type(progress.last_tick_at) == "string" and progress.last_tick_at ~= "" then
        out.last_tick_at = progress.last_tick_at
    end
    if progress.cursor ~= nil then out.cursor = progress.cursor end
    return out, nil
end

-- read_config returns the FULL editable install config a reconfigurable type
-- stores as its own private state (code/tool_ids/trigger/…) so an editor can
-- repopulate the create view on re-open. WRITE-gated and limited to types that
-- declare meta.reconfigurable, so only opt-in types expose their private config;
-- the live component title overlays state.title so a prior rename survives edit.
-- An exportable type also carries its verbatim install declaration under
-- PORTABLE_INPUT_KEY: the create view speaks that shape, and it is the same
-- declaration export_artifact ships, so an editor never reconstructs it from
-- normalized state.
function M.read_config(component_id: string): (Map?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local registry = dependencies.registry :: automation_types.RegistryModule
    local component = dependencies.component :: automation_types.ComponentModule

    local rows, query_err = component.query({
        component_ids = { component_id },
        include = { meta = true },
        access_mask = component.ACCESS.WRITE,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then return nil, "automation not found" end

    if row.impl_id == AUTOMATION_BINDING_KIND then
        local binding_raw, binding_err = M.get_binding(component_id)
        if binding_err or type(binding_raw) ~= "table" then return nil, binding_err or "automation not found" end
        local binding = binding_raw :: Binding
        local config = binding_config(binding)
        local title, missing = destination_name(binding.flow_ref)
        if title then config.flow_title = title end
        if missing then config.flow_missing = true end
        return config, nil
    end

    local raw_entry, get_err = registry.get(row.impl_id :: string)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err or not entry or entry.kind ~= "contract.binding" then return nil, "automation type not found" end
    local meta: Map = entry.meta or {}
    if meta.type ~= AUTOMATION_TYPE then return nil, "binding is not an automation type" end
    if meta.reconfigurable ~= true then return nil, "automation type does not support reconfigure" end

    local raw_state, state_err = component.get_context(component_id, component.ACCESS.WRITE)
    if state_err or type(raw_state) ~= "table" then return nil, state_err or "automation not found" end
    local context = raw_state :: Map
    local out: Map = copy_private_state(context, true) :: Map
    local live_title = row.meta and (row.meta :: Map).title
    if type(live_title) == "string" and live_title ~= "" then out.title = live_title end

    if meta.exportable == true and type(context[PORTABLE_INPUT_KEY]) == "table" then
        local portable: Map = {}
        for key, value in pairs(context[PORTABLE_INPUT_KEY] :: Map) do portable[key] = value end
        if type(live_title) == "string" and live_title ~= "" then portable.title = live_title end
        out[PORTABLE_INPUT_KEY] = portable
    end
    return out, nil
end

local function is_array(value: Map): boolean
    local count = 0
    for key, _ in pairs(value) do
        if type(key) ~= "number" or key < 1 or key ~= math.floor(key) then return false end
        count = count + 1
    end
    for index = 1, count do
        if value[index] == nil then return false end
    end
    return count > 0
end

local function portable_copy(value: any): any
    if type(value) ~= "table" then return value end
    local source = value :: Map
    local out: Map = {}
    if is_array(source) then
        for index, item in ipairs(source) do out[index] = portable_copy(item) end
        return out
    end
    for key, item in pairs(source) do out[key] = portable_copy(item) end
    return out
end

local function portable_merge(base: any, overrides: any): any
    if type(overrides) ~= "table" then return overrides end
    local override_map = overrides :: Map
    if is_array(override_map) then
        local copied: Map = {}
        for index, value in ipairs(override_map) do copied[index] = portable_merge(nil, value) end
        return copied
    end
    local out: Map = {}
    if type(base) == "table" and not is_array(base :: Map) then
        for key, value in pairs(base :: Map) do out[key] = portable_merge(nil, value) end
    end
    for key, value in pairs(override_map) do
        out[key] = portable_merge(out[key], value)
    end
    return out
end

local ARTIFACT_FIELDS: Map = {
    apiVersion = true,
    kind = true,
    metadata = true,
    spec = true,
}

local AUTOMATION_SPEC_FIELDS: Map = {
    type = true,
    input = true,
}

local EXACT_FLOW_BINDING_SPEC_FIELDS: Map = {
    portable_key = true,
    title = true,
    enabled = true,
    trigger = true,
    flow_ref = true,
    mapping_spec = true,
    guard_expr = true,
    execution_policy = true,
    authority_scope = true,
}

local ARTIFACT_METADATA_FIELDS: Map = {
    title = true,
}

local function reject_unknown_fields(value: Map, allowed: Map, label: string): string?
    for key, _ in pairs(value) do
        if type(key) ~= "string" or allowed[key] ~= true then
            return label .. " contains unsupported field: " .. tostring(key)
        end
    end
    return nil
end

local function is_object(value: any): boolean
    if type(value) ~= "table" then return false end
    for key, _ in pairs(value :: Map) do
        if type(key) ~= "string" then return false end
    end
    return true
end

local function validate_artifact_envelope(raw: Map): string?
    local fields_err = reject_unknown_fields(raw, ARTIFACT_FIELDS, "artifact")
    if fields_err then return fields_err end
    if raw.apiVersion ~= "kickside.automation/v1" then
        return "unsupported automation artifact apiVersion"
    end
    if type(raw.kind) ~= "string" or raw.kind == "" then
        return "artifact kind is required"
    end
    if raw.metadata ~= nil then
        if not is_object(raw.metadata) then return "artifact metadata must be an object" end
        local metadata = raw.metadata :: Map
        local metadata_err = reject_unknown_fields(metadata, ARTIFACT_METADATA_FIELDS, "artifact metadata")
        if metadata_err then return metadata_err end
        if metadata.title ~= nil and type(metadata.title) ~= "string" then
            return "artifact metadata.title must be a string"
        end
    end
    if not is_object(raw.spec) then return "artifact spec is required" end
    return nil
end

-- export_artifact returns a portable declaration, not a component snapshot.
-- Runtime identity, placement, rollback state, progress, and component ids are
-- deliberately absent. The install input remains intact, including a Data
-- Sync's transform, so the artifact can recreate the automation elsewhere.
function M.export_artifact(component_id: string): (Map?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local component = dependencies.component :: automation_types.ComponentModule
    local rows, query_err = component.query({
        component_ids = { component_id },
        include = { meta = true },
        access_mask = component.ACCESS.WRITE,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then
        return nil, "automation not found"
    end

    if row.impl_id == AUTOMATION_BINDING_KIND then
        local config_raw, config_err = M.read_config(component_id)
        if config_err or type(config_raw) ~= "table" then return nil, config_err or "automation config not found" end
        local config = config_raw :: Map
        local title = type(config.title) == "string" and config.title or ""
        local declaration = portable_copy(config) :: Map
        -- The artifact carries the portable spec only. Revision and the
        -- provider-resolved destination name are live state of this instance.
        declaration.revision = nil
        declaration.flow_title = nil
        declaration.flow_missing = nil
        return {
            apiVersion = "kickside.automation/v1",
            kind = "ExactFlowBinding",
            metadata = { title = title },
            spec = declaration,
        }, nil
    end

    local registry = dependencies.registry :: automation_types.RegistryModule
    local raw_entry, entry_err = registry.get(row.impl_id :: string)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if entry_err or not entry then return nil, entry_err or "automation type not found" end
    local entry_meta = type(entry.meta) == "table" and (entry.meta :: Map) or {}
    if entry_meta.exportable ~= true then return nil, "automation type is not exportable" end
    local state, state_err = component.get_context(component_id, component.ACCESS.WRITE)
    if state_err then return nil, state_err end
    local portable = type(state) == "table" and (state :: Map)[PORTABLE_INPUT_KEY] or nil
    if type(portable) ~= "table" then return nil, "automation has no portable install input" end
    local config = portable_copy(portable :: Map) :: Map
    local title = type((config :: Map).title) == "string" and (config :: Map).title or ""
    return {
        apiVersion = "kickside.automation/v1",
        kind = "Automation",
        metadata = { title = title },
        spec = {
            type = row.impl_id,
            input = config :: Map,
        },
    }, nil
end

-- import_artifact is the inverse of export_artifact. Placement is an explicit
-- destination concern and therefore is supplied separately rather than being
-- embedded in the portable artifact.
function M.import_artifact(artifact: any, parent_id: string?, input_overrides: any?): (Map?, any)
    if type(artifact) ~= "table" then return nil, "artifact must be an object" end
    local raw = artifact :: Map
    local envelope_err = validate_artifact_envelope(raw)
    if envelope_err then return nil, envelope_err end
    local spec = raw.spec :: Map
    if raw.kind == "Automation" then
        local spec_err = reject_unknown_fields(spec, AUTOMATION_SPEC_FIELDS, "automation artifact spec")
        if spec_err then return nil, spec_err end
        if type(spec.type) ~= "string" or spec.type == "" then
            return nil, "automation artifact spec.type is required"
        end
        if not is_object(spec.input) then return nil, "automation artifact spec.input is required" end
        if input_overrides ~= nil and not is_object(input_overrides) then
            return nil, "input_overrides must be an object"
        end
        local install_input = input_overrides ~= nil
            and portable_merge(spec.input, input_overrides)
            or portable_merge(nil, spec.input)
        local installed, install_err = M.install_type(spec.type :: string, install_input, parent_id)
        if install_err or not installed then return nil, install_err or "automation import failed" end
        return {
            id = installed.id,
            type = installed.type,
            created = installed.created,
        }, nil
    end
    if raw.kind == "ExactFlowBinding" then
        local spec_err = reject_unknown_fields(spec, EXACT_FLOW_BINDING_SPEC_FIELDS, "exact flow binding artifact spec")
        if spec_err then return nil, spec_err end
        if input_overrides ~= nil then
            return nil, "input_overrides are not supported for ExactFlowBinding artifacts"
        end
        local binding, binding_err = M.create_binding(spec, parent_id)
        if binding_err or not binding then return nil, binding_err or "binding import failed" end
        return { id = binding.binding_id, type = AUTOMATION_BINDING_KIND, created = true }, nil
    end
    return nil, "unsupported automation artifact kind"
end

function M.patch_state(component_id: string, patch: any, opts: automation_types.PatchStateOptions?): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(patch) ~= "table" then return nil, "patch must be a table" end
    if (patch :: Map)._rollback ~= nil then
        return nil, "invalid patch: _rollback is managed by the automation runtime"
    end
    if (patch :: Map).component_id ~= nil then
        return nil, "invalid patch: component_id is managed by the automation runtime"
    end
    if (patch :: Map).public_state ~= nil then
        return nil, "invalid patch: public_state is managed by component public meta"
    end
    if (patch :: Map)[EXECUTION_IDENTITY_KEY] ~= nil then
        return nil, "invalid patch: " .. EXECUTION_IDENTITY_KEY .. " is managed by the automation runtime"
    end
    if (patch :: Map)[TRIGGER_STATE_KEY] ~= nil then
        return nil, "invalid patch: " .. TRIGGER_STATE_KEY .. " is managed by the trigger service"
    end
    if (patch :: Map)[TRIGGER_PROGRESS_KEY] ~= nil then
        return nil, "invalid patch: " .. TRIGGER_PROGRESS_KEY .. " is managed by the trigger service"
    end
    if (patch :: Map)[PORTABLE_INPUT_KEY] ~= nil then
        return nil, "invalid patch: portable install input is managed by the automation runtime"
    end
    if opts and opts.public_meta ~= nil and type(opts.public_meta) ~= "table" then
        return nil, "public_meta must be a table"
    end
    if opts and opts.portable_input_patch ~= nil and type(opts.portable_input_patch) ~= "table" then
        return nil, "portable install input patch must be a table"
    end

    local component = dependencies.component :: automation_types.ComponentModule

    local delete_keys = opts and type(opts.delete_keys) == "table" and opts.delete_keys or nil
    local has_deletes = type(delete_keys) == "table" and next(delete_keys :: { string }) ~= nil
    -- Read to preserve the returned full sanitized state contract. The no-delete
    -- write below still sends only PATCH_CONTEXT, never this snapshot.
    local current, read_err = component.get_context(component_id, component.ACCESS.WRITE)
    if read_err then return nil, read_err end
    local rollback: any = nil
    local frozen_identity: any = nil
    if type(current) == "table" then
        rollback = (current :: Map)._rollback
        frozen_identity = (current :: Map)[EXECUTION_IDENTITY_KEY]
    end
    local write_patch: Map = {}
    for key, value in pairs(patch :: Map) do write_patch[key] = value end

    -- A product never writes the engine-owned portable declaration directly.
    -- When its config and durable state change together, however, split writes
    -- leave a restart-visible half transition. Fold the declaration here and
    -- issue it in the same PATCH_CONTEXT command as the product patch.
    if opts and opts.portable_input_patch ~= nil then
        local registry = dependencies.registry :: automation_types.RegistryModule
        local rows, query_err = component.query({
            component_ids = { component_id },
            access_mask = component.ACCESS.WRITE,
        })
        if query_err then return nil, query_err end
        local row = rows and rows[1]
        if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then return nil, "automation not found" end
        local raw_entry, get_err = registry.get(row.impl_id :: string)
        local entry = raw_entry :: automation_types.RegistryEntry?
        if get_err or not entry then return nil, get_err or "automation type not found" end
        local stored = type(current) == "table" and (current :: Map)[PORTABLE_INPUT_KEY] or nil
        local merged = portable_merge(type(stored) == "table" and stored or nil, opts.portable_input_patch)
        local input_err = validate_install_input((entry.meta or {}).inputs, merged)
        if input_err then return nil, input_err end
        write_patch[PORTABLE_INPUT_KEY] = portable_copy(merged)
    end

    local next_state = copy_private_state(current, true, true)

    for k, v in pairs(write_patch) do
        next_state[k] = v
    end
    if has_deletes then
        for _, key in ipairs(delete_keys :: { string }) do
            if type(key) == "string" and key ~= "_rollback" and key ~= "component_id"
                and key ~= EXECUTION_IDENTITY_KEY and key ~= TRIGGER_STATE_KEY
                and key ~= TRIGGER_PROGRESS_KEY and key ~= PORTABLE_INPUT_KEY then
                next_state[key] = nil
            end
        end
    end
    if rollback ~= nil then next_state._rollback = rollback end
    if frozen_identity ~= nil then next_state[EXECUTION_IDENTITY_KEY] = frozen_identity end

    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local commands: { any }
    if has_deletes then
        commands = { { type = "SET_CONTEXT", payload = { private_context = next_state } } }
    else
        commands = { { type = "PATCH_CONTEXT", payload = { patch = write_patch } } }
    end
    if opts and type(opts.public_meta) == "table" and next(opts.public_meta) ~= nil then
        commands[#commands + 1] = { type = "SET_META", payload = { fields = opts.public_meta } }
    end
    local result, update_err = svc:update({
        component_id = component_id,
        commands = commands,
    })
    if update_err then return nil, update_err end
    if not result or not result.success then
        return nil, tostring(result and result.error or "state update failed")
    end
    return { success = true, id = component_id, state = copy_private_state(next_state, true) }, nil
end

-- update_portable_input is the single write path for the declaration exported
-- by an exportable Automation kind. Kind reconfiguration calls it only after
-- its live state has accepted the same input, keeping export/import aligned
-- with what is actually running without exposing the field through patch_state.
-- The declaration is held to the type's own meta.inputs here, the same contract
-- install_type enforces, so an exported artifact is installable by construction.
-- merge_portable_input folds an edit's fields over the STORED declaration and
-- persists the merged whole. A reconfigure caller sends only what it changed;
-- replacing the declaration wholesale with that partial input is how exported
-- artifacts silently lost field maps and backfill windows. Arrays replace,
-- maps merge -- the same law portable_merge applies to import overrides.
function M.merge_portable_input(component_id: string, patch: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(patch) ~= "table" then return nil, "portable install input patch must be a table" end
    local component = dependencies.component :: automation_types.ComponentModule
    local ctx, ctx_err = component.get_context(component_id, component.ACCESS.WRITE)
    if ctx_err then return nil, ctx_err end
    local stored = type(ctx) == "table" and (ctx :: Map)[PORTABLE_INPUT_KEY] or nil
    local merged = portable_merge(type(stored) == "table" and (stored :: Map) or nil, patch)
    return M.update_portable_input(component_id, merged)
end

function M.update_portable_input(component_id: string, input: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(input) ~= "table" then return nil, "portable install input must be a table" end
    local component = dependencies.component :: automation_types.ComponentModule
    local _, access_err = component.get_context(component_id, component.ACCESS.WRITE)
    if access_err then return nil, access_err end

    local registry = dependencies.registry :: automation_types.RegistryModule
    local rows, query_err = component.query({
        component_ids = { component_id },
        access_mask = component.ACCESS.WRITE,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then return nil, "automation not found" end
    local raw_entry, get_err = registry.get(row.impl_id :: string)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err or not entry then return nil, get_err or "automation type not found" end
    local input_err = validate_install_input((entry.meta or {}).inputs, input)
    if input_err then return nil, input_err end

    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local result, update_err = svc:update({
        component_id = component_id,
        commands = {
            { type = "PATCH_CONTEXT", payload = { patch = { [PORTABLE_INPUT_KEY] = portable_copy(input :: Map) } } },
        },
    })
    if update_err or not result or result.success ~= true then
        return nil, tostring(update_err or (result and result.error) or "portable install input update failed")
    end
    return result, nil
end

-- write_trigger_state is the single writer for the engine-reserved _trigger key:
-- the trigger service (kickside.automation.trigger) stores its bookkeeping block
-- ({ v, spec, mode, registration, phase }) here, and nothing else touches the key
-- (patch_state refuses it in both patch and delete_keys). block = nil clears the
-- key (trigger uninstall). Every other private-context field, the rollback chain,
-- and the frozen identity pass through verbatim.
function M.write_trigger_state(component_id: string, block: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if block ~= nil and type(block) ~= "table" then return nil, "trigger block must be a table or nil" end

    local component = dependencies.component :: automation_types.ComponentModule

    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local command: any
    if block ~= nil then
        command = { type = "PATCH_CONTEXT", payload = { patch = { [TRIGGER_STATE_KEY] = block } } }
    else
        -- PATCH_CONTEXT intentionally cannot delete keys. Uninstall reaches this
        -- path only after the trigger machine detached, so replace the full
        -- context once to remove both lifecycle and progress state together.
        local current, read_err = component.get_context(component_id, component.ACCESS.WRITE)
        if read_err then return nil, read_err end
        local next_state = copy_private_state(current, false)
        next_state[TRIGGER_STATE_KEY] = nil
        next_state[TRIGGER_PROGRESS_KEY] = nil
        command = { type = "SET_CONTEXT", payload = { private_context = next_state } }
    end
    local result, update_err = svc:update({
        component_id = component_id,
        commands = { command },
    })
    if update_err then return nil, update_err end
    if not result or not result.success then
        return nil, tostring(result and result.error or "trigger state update failed")
    end
    return { success = true, id = component_id }, nil
end

-- Poll progress is an independent top-level owner. PATCH_CONTEXT merges this
-- key transactionally without touching lifecycle control in `_trigger` or any
-- product-owned Sync state.
function M.write_trigger_progress(component_id: string, progress: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(progress) ~= "table" then return nil, "trigger progress must be a table" end
    local component = dependencies.component :: automation_types.ComponentModule
    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local result, update_err = svc:update({
        component_id = component_id,
        commands = {
            { type = "PATCH_CONTEXT", payload = { patch = { [TRIGGER_PROGRESS_KEY] = progress } } },
        },
    })
    if update_err then return nil, update_err end
    if not result or not result.success then
        return nil, tostring(result and result.error or "trigger progress update failed")
    end
    return { success = true, id = component_id }, nil
end

function M.write_trigger_install_state(component_id: string, block: any, progress: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(block) ~= "table" then return nil, "trigger block must be a table" end
    local component = dependencies.component :: automation_types.ComponentModule
    local patch: Map = { [TRIGGER_STATE_KEY] = block }
    if type(progress) == "table" then patch[TRIGGER_PROGRESS_KEY] = progress end
    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local result, update_err = svc:update({
        component_id = component_id,
        commands = { { type = "PATCH_CONTEXT", payload = { patch = patch } } },
    })
    if update_err then return nil, update_err end
    if not result or not result.success then
        return nil, tostring(result and result.error or "trigger install state update failed")
    end
    return { success = true, id = component_id }, nil
end

-- install_type: run the binding's installable.install via funcs:call (under
-- the calling actor), then persist the resulting umbrella component. On any
-- post-install failure the rollback chain is replayed.
function M.install_type(type_id: string, input: any, parent_id: string?): (automation_types.InstallTypeResult?, any)
    if type(type_id) ~= "string" or type_id == "" then return nil, "type is required" end
    -- Optional workspace placement: when the install runs from the workspace "+"
    -- popup it carries the selected folder id, so the automation component lands
    -- under it; omitted (nil) keeps the root placement the automations page uses.
    local placement_parent_id: string? = nil
    if type(parent_id) == "string" and parent_id ~= "" then placement_parent_id = parent_id end
    local registry = dependencies.registry :: automation_types.RegistryModule
    local funcs = dependencies.funcs :: automation_types.FuncsModule
    local component = dependencies.component :: automation_types.ComponentModule

    local raw_entry, get_err = registry.get(type_id)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err or not entry or entry.kind ~= "contract.binding" then
        return nil, "automation type not found"
    end
    local entry_meta: Map = entry.meta or {}
    if entry_meta.type ~= AUTOMATION_TYPE then
        return nil, "binding is not an automation type"
    end
    local input_err = validate_install_input(entry_meta.inputs, input)
    if input_err then return nil, input_err end

    -- Freeze the installer's identity before running the type body, so any
    -- event-driven runtime can later run this automation as its installer.
    local execution_identity, identity_err = capture_execution_identity()
    if identity_err or not execution_identity then return nil, identity_err end

    local install_target = M.find_method(entry, "install")
    if not install_target then
        return nil, "type does not declare installable.install"
    end

    -- Pre-allocate the component id so the install body can orchestrate against
    -- it through plain contracts (grant access, create child resources, …); the
    -- id rides in the call context and whatever the body does is undone via the
    -- recorder if registration fails. Recurring work is declared, not imperative:
    -- the body returns a `schedules` array and the engine creates each schedule
    -- after registration (when the authoritative id exists), threading the same
    -- rollback recorder and writing every created schedule_id back into state.
    local component_id = uuid.v7()

    -- funcs.new():call inherits the calling actor from this frame, so the body
    -- runs with the caller's authority; with_context exposes the component id.
    local executor, exec_err = funcs.new()
    if exec_err or not executor then return nil, "executor: " .. tostring(exec_err) end

    local contextual, context_err = (executor :: any):with_context({ component_id = component_id })
    if context_err or not contextual then
        return nil, "executor context: " .. tostring(context_err or "unavailable")
    end
    local result, call_err = (contextual :: any):call(
        install_target :: string, (type(input) == "table" and input or {}) :: Map)
    if call_err ~= nil then return nil, call_err end
    local shape_err = validate_result(result)
    if shape_err ~= nil then return nil, shape_err end

    local result_t: automation_types.InstallResult = result :: automation_types.InstallResult
    local state: Map = result_t.state
    local metadata: Map = type(result_t.metadata) == "table" and result_t.metadata or {}
    local public_state: Map? = type(result_t.public_state) == "table" and result_t.public_state or nil
    local public_state_err = validate_public_state(entry_meta.public_state_schema, public_state)
    if public_state_err then return nil, public_state_err end
    local rollback, rollback_err = normalize_rollback_chain(result_t.rollback)
    if rollback_err or not rollback then return nil, rollback_err end
    local rollback_chain = rollback :: { automation_types.RollbackStep }

    local kickside_component_meta: Map = (safe_component_metadata(metadata, "Untitled") :: Map)
    -- class stored as TEXT; flatten an array class to its first (canonical) value.
    local classes = class_array_from_meta(entry_meta)
    if #classes > 0 then kickside_component_meta.class = classes[1] end
    if public_state ~= nil then
        for k, v in pairs(public_state :: Map) do
            kickside_component_meta[k] = v
        end
    end

    local private_context: automation_types.AutomationPrivateContext = {}
    for k, v in pairs(state) do private_context[k] = v end
    private_context.component_id = component_id
    private_context._rollback = rollback_chain
    private_context[EXECUTION_IDENTITY_KEY] = execution_identity
    if entry_meta.exportable == true then
        private_context[PORTABLE_INPUT_KEY] = portable_copy(type(input) == "table" and input or {})
    end

    local svc, svc_err = component.get_service()
    if svc_err or not svc then
        M.replay(rollback_chain)
        return nil, "component service: " .. tostring(svc_err)
    end
    local reg_result, reg_err = svc:register({
        component_id = component_id,
        impl_id = type_id,
        private_context = private_context,
        meta = kickside_component_meta,
        parent_id = placement_parent_id,
    })
    if reg_err or not reg_result or not reg_result.component_id then
        M.replay(rollback_chain)
        return nil, "component registration failed: " .. tostring(reg_err)
    end
    -- The service returns the authoritative id; every schedule and the persisted
    -- state bind to it, not to the pre-allocated value.
    local final_id = reg_result.component_id :: string
    state.component_id = final_id
    private_context.component_id = final_id

    -- Create the body's declared schedules now that the component row exists.
    -- create_action_schedule records its own delete-rollback through the recorder,
    -- so the chain grows in place and a failed schedule (or any later step) tears
    -- the whole install back down. Each created schedule_id is written into the
    -- body-chosen state_key and accumulated under schedule_ids.
    local schedule_recorder: automation_types.InstallRecorder = {
        rollback = function(target_id: string, args: Map?)
            if type(target_id) == "string" and target_id ~= "" then
                rollback_chain[#rollback_chain + 1] = {
                    target = target_id,
                    args = type(args) == "table" and (args :: Map) or {},
                }
            end
        end,
    }
    local schedule_ids: { string } = {}
    local created_schedules: { Map } = {}
    local declared_schedules: { automation_types.ScheduleSpec } =
        type(result_t.schedules) == "table" and result_t.schedules or {}
    for _, spec in ipairs(declared_schedules) do
        local opts: Map = {}
        for k, v in pairs(spec :: Map) do opts[k] = v end
        opts.component_id = final_id
        opts.state_key = nil
        local schedule_module = dependencies.automation_schedule :: Map?
        local created, sched_err = (schedule_module.create_action_schedule :: any)(opts, schedule_recorder)
        if sched_err or not created or not created.schedule_id then
            M.replay(rollback_chain)
            return nil, "schedule setup failed: " .. tostring(sched_err)
        end
        local created_map = created :: Map
        local schedule_id = created_map.schedule_id :: string
        schedule_ids[#schedule_ids + 1] = schedule_id
        created_schedules[#created_schedules + 1] = created_map
        local state_key = spec.state_key
        if type(state_key) == "string" and state_key ~= "" then
            state[state_key :: string] = schedule_id
        end
    end

    -- Persist the post-schedule state once, when schedules ran: the created ids,
    -- the schedule_ids index, and the appended delete-rollback steps. With no
    -- declared schedules the registration above already holds the final state.
    if #schedule_ids > 0 then
        state.schedule_ids = schedule_ids
        for k, v in pairs(state) do private_context[k] = v end
        private_context._rollback = rollback_chain
        private_context[EXECUTION_IDENTITY_KEY] = execution_identity

        local update_result, update_err = svc:update({
            component_id = final_id,
            commands = {
                { type = "SET_CONTEXT", payload = { private_context = private_context } },
            },
        })
        if update_err or not update_result or not update_result.success then
            M.replay(rollback_chain)
            return nil, "schedule state persist failed: " .. tostring(update_err or (update_result and update_result.error))
        end
    end

    return ({
        id = final_id,
        type = type_id,
        metadata = copy_map(kickside_component_meta),
        created = type(result_t.created) == "table" and result_t.created or nil,
        schedule_ids = schedule_ids,
        schedules = created_schedules,
    } :: automation_types.InstallTypeResult), nil
end

-- reconfigure edits an installed automation IN PLACE. It resolves the type,
-- confirms it opted into reconfigure (meta.reconfigurable), and dispatches the type's
-- own `reconfigure` action through the shared call_action seam. The type applies the
-- change WITHOUT tearing down its trigger registration -- component-class schedules
-- are owned by the component lifecycle and cannot be deleted mid-life -- so config
-- edits (code, tools, title) and in-place schedule-cadence changes persist, while a
-- structural trigger change the type cannot apply in place is rejected by the type
-- with a clear error. Types that never declare meta.reconfigurable are unreachable.
function M.reconfigure(component_id: string, input: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local registry = dependencies.registry :: automation_types.RegistryModule
    local component = dependencies.component :: automation_types.ComponentModule
    local rows, query_err = component.query({
        component_ids = { component_id },
        include = { meta = true },
        access_mask = component.ACCESS.WRITE,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then return nil, "automation not found" end
    local raw_entry, get_err = registry.get(row.impl_id :: string)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err or not entry or entry.kind ~= "contract.binding" then return nil, "automation type not found" end
    local entry_meta: Map = entry.meta or {}
    if entry_meta.type ~= AUTOMATION_TYPE and entry_meta.type ~= AUTOMATION_BINDING_META_TYPE then
        return nil, "binding is not an automation type"
    end
    if entry_meta.reconfigurable ~= true then return nil, "automation type does not support reconfigure" end
    return M.call_action(component_id, "reconfigure", type(input) == "table" and input or {})
end

-- uninstall: replay the persisted rollback chain, then delete the umbrella
-- component (which dispatches deletable.delete on the binding).
--
-- options (optional table) is forwarded to every cleanup step as
-- args._delete_options. Bindings declare which keys they understand via
-- meta.delete_options (so the frontend can render a per-type confirm UI).
function M.uninstall(component_id: string, options: any): (automation_types.UninstallResult?, any)
    if type(component_id) ~= "string" or component_id == "" then
        return nil, "id is required"
    end
    local component = dependencies.component :: automation_types.ComponentModule

    -- System read of the umbrella's private context (holds the _rollback chain).
    -- Access is enforced separately by the DELETE gate below, so this read is the
    -- trusted system path rather than the actor-filtered read-port.
    local pc, read_err = component.get_private_context(component_id)
    if read_err or not pc then return nil, "automation not found" end

    -- The rollback replay below is destructive: it tears down the child KBs,
    -- threads, schedules, projections, and other resources this automation created. Gate it on DELETE
    -- access to the umbrella BEFORE replaying, so a caller who cannot delete the
    -- automation never triggers teardown. delete_component would otherwise
    -- reject only after the children are already gone, leaving orphans. This
    -- protects every entry point (HTTP, MCP action, owned-resource teardown),
    -- not just the HTTP wrapper.
    local _, access_err = component.validate_access(component_id, component.ACCESS.DELETE)
    if access_err then
        return nil, "access denied: " .. tostring(access_err)
    end

    local private_context = pc :: automation_types.AutomationPrivateContext
    local rollback: { automation_types.RollbackStep } = private_context._rollback or {}
    local replay_opts: automation_types.ReplayOptions? = nil
    if type(options) == "table" then
        replay_opts = { options = options :: Map }
    end
    local replay_count, replay_failed = M.replay(rollback, replay_opts)

    -- A step hard-failed: a child resource was not torn down. Keep the umbrella
    -- and its rollback chain so the orphan stays tracked and a retry can finish
    -- teardown, rather than deleting the only record that points at it.
    if replay_failed and replay_failed > 0 then
        return nil, "rollback incomplete: " .. tostring(replay_failed) ..
            " step(s) failed; automation retained for retry"
    end

    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local del_result, del_err = svc:delete({ component_id = component_id })
    if not del_result or not del_result.success then
        return nil, "delete_component: " .. tostring(del_err or
            (del_result and (del_result :: any).error) or "no result")
    end

    return ({
        id = component_id,
        replayed = replay_count,
        replay_failed = replay_failed,
    } :: automation_types.UninstallResult), nil
end

-- delete_automation is the shared API dispatch point. Binding artifacts do not
-- carry the installable Automation component rollback ledger, so they enter their own
-- component-kind teardown; installable Automation components retain uninstall's
-- rollback-before-unregister behavior unchanged.
function M.delete_automation(component_id: string, options: any): (Map?, any)
    local id = trim(component_id)
    if id == "" then return nil, "id is required" end
    local component = dependencies.component :: automation_types.ComponentModule

    local rows, query_err = component.query({
        component_ids = { id },
        include = { meta = true },
        access_mask = component.ACCESS.DELETE,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row then return nil, "automation not found: " .. id end

    if (row :: automation_types.ComponentRow).impl_id == AUTOMATION_BINDING_KIND then
        local deleted, delete_err = M.delete_binding(id)
        if delete_err or type(deleted) ~= "table" then return nil, delete_err or "binding delete failed" end
        return deleted :: Map, nil
    end
    local uninstalled, uninstall_err = M.uninstall(id, options)
    if uninstall_err or type(uninstalled) ~= "table" then return nil, uninstall_err or "automation uninstall failed" end
    return uninstalled :: Map, nil
end

local function owned_resource_state_keys(meta: Map, resource_kind: string): { string }
    local out: { string } = {}
    local specs = meta.owned_resources
    if type(specs) ~= "table" then return out end
    for _, spec in ipairs(specs :: { any }) do
        if type(spec) == "table" then
            local s = spec :: automation_types.OwnedResourceSpec
            if s.kind == resource_kind and type(s.state_key) == "string" and s.state_key ~= "" then
                out[#out + 1] = s.state_key
            end
        end
    end
    return out
end

-- find_owner_by_resource: locate the umbrella automation instance that declares
-- ownership of `resource_kind` and whose private state contains `resource_id`
-- under the declared state key. Resource modules ask by kind/id only; private
-- state key names are owned by automation type metadata (`meta.owned_resources`).
function M.find_owner_by_resource(resource_kind: string, resource_id: string): (string?, any)
    if type(resource_kind) ~= "string" or resource_kind == "" then return nil, nil end
    if type(resource_id) ~= "string" or resource_id == "" then return nil, nil end

    local registry = dependencies.registry :: automation_types.RegistryModule
    local entries, entries_err = registry.find({
        [".kind"] = "contract.binding",
        ["meta.type"] = AUTOMATION_TYPE,
    })
    if entries_err then return nil, entries_err end

    local impl_ids: { string } = {}
    local keys_by_impl: { [string]: { string } } = {}
    for _, entry in ipairs(entries or {}) do
        local typed_entry = entry :: automation_types.RegistryEntry
        local id = typed_entry.id
        local keys = owned_resource_state_keys(typed_entry.meta or {}, resource_kind)
        if type(id) == "string" and id ~= "" and #keys > 0 then
            impl_ids[#impl_ids + 1] = id
            keys_by_impl[id] = keys
        end
    end
    if #impl_ids == 0 then return nil, nil end

    local component = dependencies.component :: automation_types.ComponentModule

    -- System-wide reverse lookup across every owner's umbrellas: a trusted
    -- (unscoped) read, since teardown must find the owner regardless of caller.
    local components = (component.list_system({
        impl_ids = impl_ids,
        include = { private_context = true },
    }) or {}) :: { automation_types.ComponentRow }
    for _, c in ipairs(components or {}) do
        local pc: Map = c.private_context or {}
        local keys = keys_by_impl[c.impl_id]
        if keys then
            for _, key in ipairs(keys) do
                if pc[key] == resource_id then
                    return tostring(c.component_id), nil
                end
            end
        end
    end
    return nil, nil
end

-- find_owners_by_resource: every automation instance that cannot outlive the
-- named resource — the umbrellas declaring ownership of it, plus the bindings
-- addressed at it.
function M.find_owners_by_resource(resource_kind: string, resource_id: string): ({ string }, any)
    if type(resource_kind) ~= "string" or resource_kind == "" then return {}, nil end
    if type(resource_id) ~= "string" or resource_id == "" then return {}, nil end

    local out: { string } = {}
    local seen: { [string]: boolean } = {}
    local owner_id, owner_err = M.find_owner_by_resource(resource_kind, resource_id)
    if owner_err then return {}, owner_err end
    if owner_id and owner_id ~= "" then
        out[#out + 1] = owner_id
        seen[owner_id] = true
    end
    local bindings, bindings_err = bindings_for_destination(resource_kind, resource_id)
    if bindings_err then return {}, bindings_err end
    for _, id in ipairs(bindings or {}) do
        if not seen[id] then
            out[#out + 1] = id
            seen[id] = true
        end
    end
    return out, nil
end

-- call_action: dispatch a binding method declared in meta.actions.
-- component.open auto-loads private_context as ctx for the dispatched method.
function M.call_action(component_id: string, method_name: string, args: any): (automation_types.CallActionResult?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(method_name) ~= "string" or method_name == "" then return nil, "method is required" end
    if M.is_lifecycle(method_name) then
        return nil, "lifecycle method '" .. method_name .. "' is not callable as an action"
    end
    local component = dependencies.component :: automation_types.ComponentModule

    local required_access = action_required_access(component, method_name)
    local component_rows, access_err = component.query({
        component_ids = { component_id },
        access_mask = required_access,
    })
    local component_row = component_rows and component_rows[1]
    if access_err or not component_row or not component_row.impl_id then
        return nil, "automation not found"
    end

    -- The allowlist reflects the CURRENT type declaration, served from the live
    -- composition: presence keyed by impl id already proves the entry is an
    -- automation of an accepted meta.type, and the resolved action carries the
    -- fresh { contract, target } even when a per-process registry.get is stale.
    local view, view_err = type_action_view(component_row.impl_id :: string)
    if view_err then return nil, "binding lookup failed: " .. tostring(view_err) end
    if not view then return nil, "component is not an automation" end
    local action = view.actions[method_name]
    if not action then
        return nil, "method '" .. method_name .. "' is not declared as a public automation action"
    end

    local action_contract = trim(action.contract)
    if action_contract == "" then return nil, "action contract is not declared" end
    local instance, open_err = component.open(component_id, required_access, action_contract)
    if open_err or not instance then
        return nil, "open failed: " .. tostring(open_err)
    end
    local fn = instance[method_name]
    if type(fn) ~= "function" then
        return nil, "method '" .. method_name .. "' could not be resolved on instance"
    end
    local result, call_err = fn(instance, type(args) == "table" and args or {})
    if call_err ~= nil then return nil, call_err end
    return ({ id = component_id, method = method_name, result = result }) :: automation_types.CallActionResult, nil
end

return M :: Engine
end

return { new = new_engine }
