-- Install runner for kickside.automation types.
--
-- An install body is a closure that creates whatever the automation needs.
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

local M = {}
local automation_types = require("types")
local execution_identity = require("execution_identity")
local failure_lib = require("automation_failure")
local stuck_item = require("stuck_item")
local uuid = require("uuid")
local json = require("json")

-- Every module the engine uses is DECLARED on the `lib` entry (modules/imports) and
-- required here at load -- no runtime require, no undeclared lookup. _declared is the
-- single source of those handles; tests inject stubs through M._modules (the
-- with_loaded_modules seam) and mod() is the one explicit resolution point that prefers
-- a test stub over the declared module.
M._modules = {} :: { [string]: any }

local _declared: { [string]: any } = {
    contract = require("contract"),
    component = require("component"),
    registry = require("registry"),
    funcs = require("funcs"),
    logger = require("logger"),
    security = require("security"),
    autoinit = require("autoinit"),
    automation_schedule = require("automation_schedule"),
    view_components = require("view_components"),
    sql = require("sql"),
    time = require("time"),
    expr = require("expr"),
    ctx = require("ctx"),
    workflow_store = require("workflow_store"),
    workflow_published_catalog = require("workflow_published_catalog"),
    workflow_ref = require("workflow_ref"),
}

local function mod(name: string): any
    return M._modules[name] or _declared[name]
end

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function copy_map(raw: any): automation_types.Map
    local out: automation_types.Map = {}
    if type(raw) ~= "table" then return out end
    for k, v in pairs(raw :: automation_types.Map) do out[k] = v end
    return out
end

local function rollback_logger(): automation_types.LoggerInstance?
    local logger_mod = mod("logger")
    if logger_mod then return (logger_mod :: any):named("automations.rollback") :: automation_types.LoggerInstance end
    return nil
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
local function capture_execution_identity(): (automation_types.Map?, string?)
    local id, err = execution_identity.capture("automation")
    if err or not id then return nil, err or "could not capture execution identity" end
    local row, row_err = execution_identity.to_row(id)
    if row_err or not row then return nil, row_err or "could not serialize execution identity" end
    return row :: automation_types.Map, nil
end
M._capture_execution_identity = capture_execution_identity

local function pagination_int(value: any, default_value: integer, min_value: integer, max_value: integer?, name: string): (integer?, string?)
    if value == nil then return default_value, nil end
    if type(value) ~= "number" or value ~= math.floor(value) then
        return nil, name .. " must be an integer"
    end

    local n = value :: integer
    if n < min_value then return nil, name .. " must be >= " .. tostring(min_value) end
    if max_value ~= nil and n > max_value then return nil, name .. " must be <= " .. tostring(max_value) end
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
local function default_runner(target: string, args: automation_types.Map): any
    local funcs = mod("funcs") :: automation_types.FuncsModule?
    if not funcs then
        error("automations.rollback: funcs module not declared")
    end
    local executor, err = funcs.new()
    if err or not executor then
        error("automations.rollback: funcs.new failed: " .. tostring(err))
    end
    local _, call_err = executor:call(target, args)
    if call_err then
        local lg = rollback_logger()
        if lg then lg:warn("rollback call failed", { target = target, error = tostring(call_err) }) end
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
                if lg then
                    lg:warn("rollback runner raised", {
                        target = entry.target, error = tostring(ret),
                    })
                end
            elseif ret ~= nil then
                failed = failed + 1
                local lg = rollback_logger()
                if lg then
                    lg:warn("rollback step returned error", {
                        target = entry.target, error = tostring(ret),
                    })
                end
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
    local max_index = 0
    local count = 0
    for k, spec in pairs(raw :: automation_types.Map) do
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
        local s = spec :: automation_types.Map
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
        if k > max_index then max_index = k end
    end
    if max_index ~= count then
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
        local max_index = 0
        local count = 0
        for k, v in pairs(result.created :: automation_types.Map) do
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
            if k > max_index then max_index = k end
        end
        if max_index ~= count then
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
    local max_index = 0
    local count = 0
    for k, step in pairs(raw :: automation_types.Map) do
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
            args = type(s.args) == "table" and (s.args :: automation_types.Map) or {},
        }
        count = count + 1
        if k > max_index then max_index = k end
    end
    if max_index ~= count then
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
        rollback = function(target_id: string, args: automation_types.Map?)
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
    local options: automation_types.Map? = type(replay_opts.options) == "table" and replay_opts.options or nil
    local effective_chain: { automation_types.RollbackStep } = source_chain
    if options ~= nil then
        local cloned: { automation_types.RollbackStep } = {}
        for i, e in ipairs(source_chain) do
            if type(e) == "table" and type(e.target) == "string" then
                local merged_args: automation_types.Map = {}
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
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return {} end
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

local function class_array_from_meta(entry_meta: automation_types.Map): {string}
    local out: {string} = {}
    if type(entry_meta.class) == "table" then
        for _, c in ipairs(entry_meta.class) do out[#out + 1] = c end
    elseif type(entry_meta.class) == "string" then
        out[#out + 1] = entry_meta.class
    end
    return out
end

local function copy_private_state(raw: any, omit_runtime: boolean?): automation_types.AutomationPrivateContext
    local out: automation_types.AutomationPrivateContext = {}
    if type(raw) ~= "table" then return out end
    for k, v in pairs(raw :: automation_types.Map) do
        if not (omit_runtime == true and (k == "component_id" or k == "_rollback" or k == EXECUTION_IDENTITY_KEY)) then
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
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return {} end
    local query: automation_types.Map = { [".kind"] = "contract.binding", ["meta.type"] = AUTOMATION_TYPE }
    if type(type_meta) == "table" then
        for k, v in pairs(type_meta :: automation_types.Map) do
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
local RESERVED_STATE_KEYS: { [string]: boolean } = {
    _rollback = true,
    public_state = true,
    component_id = true,
    [EXECUTION_IDENTITY_KEY] = true,
}

local function strip_reserved_state(raw: any): automation_types.Map
    local out: automation_types.Map = {}
    if type(raw) ~= "table" then return out end
    for k, v in pairs(raw :: automation_types.Map) do
        if not RESERVED_STATE_KEYS[k] then out[k] = v end
    end
    return out
end

local function public_schema_fields(schema: any): (any?, string?)
    if schema == nil then return nil, nil end
    if type(schema) ~= "table" then return nil, "public_state_schema must be a table" end
    local fields = (schema :: automation_types.Map).fields
    if type(fields) ~= "table" then return nil, "public_state_schema.fields must be a table" end
    return fields, nil
end

local function field_key(field: any): string?
    if type(field) ~= "table" then return nil end
    local key = (field :: automation_types.Map).key
    if type(key) ~= "string" or key == "" then return nil end
    return key
end

local function option_value(option: any): any
    if type(option) == "table" then return (option :: automation_types.Map).value end
    return option
end

local function conditional_required(field: any, public_state: any): boolean
    local rule = (field :: automation_types.Map).required_if
    if type(rule) ~= "table" then return false end
    local key = (rule :: automation_types.Map).key or (rule :: automation_types.Map).field
    if type(key) ~= "string" or key == "" then return false end
    return type(public_state) == "table" and (public_state :: automation_types.Map)[key] == (rule :: automation_types.Map).equals
end

local function validate_public_field_value(field: any, value: any, public_state: any): string?
    local key = field_key(field) or "field"
    local kind = type((field :: automation_types.Map).type) == "string" and (field :: automation_types.Map).type or "text"
    local required = (field :: automation_types.Map).required == true or conditional_required(field, public_state)

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

    if kind == "select" and type((field :: automation_types.Map).options) == "table" and #((field :: automation_types.Map).options :: { any }) > 0 then
        for _, option in ipairs(((field :: automation_types.Map).options :: { any })) do
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
        if next(public_state :: automation_types.Map) ~= nil then
            return "public_state requires meta.public_state_schema"
        end
        return nil
    end

    for key, field in pairs(allowed) do
        local err = validate_public_field_value(field, (public_state :: automation_types.Map)[key], public_state)
        if err then return err end
    end
    for key, _ in pairs(public_state :: automation_types.Map) do
        if type(key) ~= "string" or allowed[key] == nil then
            return "public_state contains undeclared field: " .. tostring(key)
        end
    end
    return nil
end

local function coerce_public_field_read_value(field: any, value: any): (any, string?)
    if value == nil then return nil, nil end
    local key = field_key(field) or "field"
    local kind = type((field :: automation_types.Map).type) == "string" and (field :: automation_types.Map).type or "text"

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

local function project_public_state(schema: any, public_state: any): (automation_types.Map?, any)
    if public_state == nil then return {}, nil end
    if type(public_state) ~= "table" then return nil, "public_state must be a table" end

    local allowed, fields_err = public_state_field_map(schema)
    if fields_err then return nil, fields_err end
    if not allowed then return {}, nil end

    local out: automation_types.Map = {}
    for key, field in pairs(allowed) do
        local value = (public_state :: automation_types.Map)[key]
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
    local kind = (schema :: automation_types.Map).type
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
        local min_length = (field :: automation_types.Map).minLength
        if type(min_length) == "number" and #value < min_length then
            return key .. " must be at least " .. tostring(min_length) .. " characters"
        end
    end

    local enum = (field :: automation_types.Map).enum
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
    local props = (schema :: automation_types.Map).properties
    if type(props) ~= "table" then return nil end

    local required = (schema :: automation_types.Map).required
    if type(required) == "table" then
        for _, key in ipairs(required :: { any }) do
            if type(key) ~= "string" or key == "" then
                return "invalid input schema: required entries must be strings"
            end
            local value = (input :: automation_types.Map)[key]
            if value == nil or value == "" then return path .. key .. " is required" end
        end
    end

    for key, field in pairs(props :: automation_types.Map) do
        if type(key) ~= "string" or key == "" then
            return "invalid input schema: property keys must be strings"
        end
        local err = validate_input_value(path .. key, field, (input :: automation_types.Map)[key])
        if err then return err end
    end

    if (schema :: automation_types.Map).additionalProperties ~= true then
        for key, _ in pairs(input :: automation_types.Map) do
            if type(key) ~= "string" or (props :: automation_types.Map)[key] == nil then
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
    local meta: automation_types.Map = entry.meta or {}
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
        limit = 1,
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
    local view_components = mod("view_components")
    if not view_components then return nil end
    local rec, _ = (view_components :: any).get(component_id)
    if type(rec) ~= "table" then return nil end
    local r = rec :: automation_types.Map
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
local function enrich_component_block(meta_component: any): automation_types.Map?
    if type(meta_component) ~= "table" then return nil end
    local out: automation_types.Map = {}
    for k, v in pairs(meta_component :: automation_types.Map) do out[k] = v end
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

local function safe_component_metadata(metadata: automation_types.Map, fallback_title: string?): any
    local out: automation_types.Map = {
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
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return nil, "registry module unavailable" end

    local entries, err = registry.find({
        [".kind"] = "contract.binding",
        ["meta.type"] = AUTOMATION_TYPE,
    })
    if err then return nil, err end

    local class_filter = opts.class
    local category_filter = opts.category

    local types: {automation_types.AutomationType} = {}
    for _, entry in ipairs(entries or {}) do
        local typed_entry = entry :: automation_types.RegistryEntry
        local meta: automation_types.Map = typed_entry.meta or {}
        if meta.announced ~= false
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
                reconfigurable = meta.reconfigurable == true,
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
-- ids. The descriptor speaks the v2 vocabulary only: `surface` is the kind,
-- `event` the events reference, `config_schema` the
-- config face, `output_schema`/`input_schema` the data faces, `operations` the
-- store ABI. No back-compat surface fields are emitted.
local PORT_META_TYPE = "kickside.automation.port"
local TRIGGER_META_TYPE = "kickside.automation.trigger"
local SCHEDULE_TRIGGER = "kickside.automation:periodic_trigger"
local EVENT_META_TYPE = "kickside.core.threads.event"
local PULLABLE_CONTRACT = "kickside.data:pullable"
local WORKFLOW_EXECUTE = "kickside.workflows:execute"
local BEHAVIOR_INVOKE = "kickside.workflows:behaviors_invoke"

-- Read a port entry's declaration field. Live registry entries carry non-standard
-- top-level yaml keys under `.data`; test fixtures may set them directly.
local function entry_field(entry: any, key: string): any
    if type(entry) == "table" then
        local data = (entry :: any).data
        if type(data) == "table" and (data :: automation_types.Map)[key] ~= nil then
            return (data :: automation_types.Map)[key]
        end
        return (entry :: automation_types.Map)[key]
    end
    return nil
end

-- The first declared class of a binding (meta.class is a string or a string list).
local function first_class(meta: automation_types.Map): string
    local cls = meta.class
    if type(cls) == "string" then return cls :: string end
    if type(cls) == "table" and type((cls :: { any })[1]) == "string" then return tostring((cls :: { any })[1]) end
    return ""
end

-- Whether a binding entry implements a data contract (non-empty method on it).
local function binding_implements(binding_entry: any, contract_id: string): boolean
    local data = type(binding_entry) == "table" and (binding_entry :: any).data or nil
    local contracts = type(data) == "table" and (data :: automation_types.Map).contracts or nil
    if type(contracts) ~= "table" then return false end
    for _, raw in ipairs(contracts :: { any }) do
        local c = type(raw) == "table" and (raw :: automation_types.Map) or {}
        if c.contract == contract_id then return true end
    end
    return false
end

-- Whether a store port's operations map declares an operation. The canonical form
-- is a map ({ upsert = {...} }); validation rejects every other encoding.
local function declares_operation(operations: any, name: string): boolean
    if type(operations) ~= "table" then return false end
    return (operations :: automation_types.Map)[name] ~= nil
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

-- Resolve the output face of an events port from its referenced event-type entry's
-- schema (the event's one home). Returns nil when the event entry cannot be read.
local function event_output_face(registry: automation_types.RegistryModule?, event_id: string): any
    if not registry or type(event_id) ~= "string" or event_id == "" then return nil end
    local entry, err = registry.get(event_id)
    if err or type(entry) ~= "table" then return nil end
    local schema = (entry :: any).schema
    if schema ~= nil then return schema end
    local data = (entry :: any).data
    if type(data) == "table" then return (data :: automation_types.Map).schema end
    return nil
end

-- The capability versions a port advertises, as a map of name -> integer. A
-- port states them; its owning binding states the fallback for a whole family of
-- ports. Values that are not versions are dropped, so a malformed declaration
-- reads as "does not advertise" rather than as an arbitrary version.
local function port_capabilities(port_entry: any, binding_meta: automation_types.Map): automation_types.Map?
    local declared = entry_field(port_entry, "capabilities")
    if type(declared) ~= "table" then declared = binding_meta.capabilities end
    if type(declared) ~= "table" then return nil end
    local out: automation_types.Map = {}
    local found = false
    for name, version in pairs(declared :: automation_types.Map) do
        local n = tonumber(version)
        if type(name) == "string" and name ~= "" and n ~= nil and n >= 0 then
            out[name] = math.floor(n)
            found = true
        end
    end
    if not found then return nil end
    return out
end

-- port_descriptor derives one catalog descriptor from a port entry and its owning
-- binding, by the ordered precedence markers > pullable backing. Returns nil + an
-- error string when the port cannot be classified or its backing is missing.
local function port_descriptor(registry: automation_types.RegistryModule?, port_entry: any, binding_index: { [string]: any }): (automation_types.Map?, string?)
    local typed_port = port_entry :: automation_types.RegistryEntry
    local port_meta: automation_types.Map = typed_port.meta or {}
    local binding = tostring(entry_field(port_entry, "binding") or "")
    if binding == "" then return nil, tostring(typed_port.id) .. " declares no binding" end
    local binding_entry = binding_index[binding]
    if binding_entry == nil then return nil, tostring(typed_port.id) .. " binding not found: " .. binding end
    local binding_meta: automation_types.Map = (type((binding_entry :: any).meta) == "table" and (binding_entry :: any).meta or {}) :: automation_types.Map

    local event = entry_field(port_entry, "event")
    local operations = entry_field(port_entry, "operations")
    local output_schema = entry_field(port_entry, "output_schema")
    local input_schema = entry_field(port_entry, "input_schema")
    local input_mode = entry_field(port_entry, "input_mode")

    local desc: automation_types.Map = {
        id = typed_port.id,
        binding = binding,
        class = first_class(binding_meta),
        config_schema = entry_field(port_entry, "config_schema"),
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
    -- The capability map rides the descriptor the catalog scan already builds:
    -- the gate reads it from here, so arming costs no extra registry scan.
    local capabilities = port_capabilities(port_entry, binding_meta)
    if capabilities then desc.capabilities = capabilities end

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
local function build_catalog(): ({automation_types.Map}?, any)
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return nil, "registry module unavailable" end
    local ports, perr = registry.find({ [".kind"] = "registry.entry", ["meta.type"] = PORT_META_TYPE })
    if perr then return nil, perr end
    local bindings, berr = registry.find({ [".kind"] = "contract.binding" })
    if berr then return nil, berr end

    local binding_index: { [string]: any } = {}
    for _, b in ipairs(bindings or {}) do
        binding_index[tostring((b :: automation_types.RegistryEntry).id or "")] = b
    end

    local out: {automation_types.Map} = {}
    for _, port_entry in ipairs(ports or {}) do
        local desc, derr = port_descriptor(registry, port_entry, binding_index)
        if desc then
            out[#out + 1] = desc
        elseif derr then
            local logger_mod = mod("logger")
            if logger_mod then
                (logger_mod :: any):named("automations.catalog"):error("port entry skipped", { error = derr })
            end
        end
    end
    table.sort(out, function(a: automation_types.Map, b: automation_types.Map) return tostring(a.title or "") < tostring(b.title or "") end)
    return out, nil
end

-- Partition the catalog by data direction, derived from the port surface: store
-- ports flow IN (sinks); collection and events ports flow OUT (sources).
local function surface_is_dir(surface: string, dir: string): boolean
    if dir == "in" then return surface == "store" end
    return surface == "collection" or surface == "events"
end

local function list_io(dir: string): ({automation_types.Map}?, any)
    local catalog, err = build_catalog()
    if err then return nil, err end
    local out: {automation_types.Map} = {}
    for _, desc in ipairs(catalog or {}) do
        if surface_is_dir(tostring((desc :: automation_types.Map).surface or ""), dir) then out[#out + 1] = desc end
    end
    return out, nil
end

-- list_sinks: every store port (surface="store"). The binding's
-- kickside.data:writable.write is the backing. Every row carries store_tier
-- ("full" | "append_only") — consumers read the computed field, not operations.
function M.list_sinks(): ({automation_types.Map}?, any)
    local list, err = list_io("in")
    if err then return nil, err end
    return (list :: {automation_types.Map}?), nil
end

-- list_sources: every source port (surface="collection" | "events"). Collection
-- sources are pulled through their kickside.data:pullable backing; events sources
-- are component thread events consumed by an automation-owned projection. Consumers
-- read the computed surface, not dir/mode.
function M.list_sources(): ({automation_types.Map}?, any)
    local list, err = list_io("out")
    if err then return nil, err end
    return (list :: {automation_types.Map}?), nil
end

local function entry_meta_table(entry: any): automation_types.Map
    if type(entry) ~= "table" then return {} end
    local meta = (entry :: any).meta
    if type(meta) == "table" then return meta :: automation_types.Map end
    local data = (entry :: any).data
    if type(data) == "table" and type((data :: automation_types.Map).meta) == "table" then
        return (data :: automation_types.Map).meta :: automation_types.Map
    end
    return {}
end

local function descriptor_id(entry: any): string
    if type(entry) ~= "table" then return "" end
    return tostring((entry :: automation_types.Map).id or "")
end

local function decode_json_table(raw: any): automation_types.Map
    if type(raw) == "table" then return raw :: automation_types.Map end
    if type(raw) ~= "string" or raw == "" then return {} end
    local decoded, err = json.decode(raw)
    if err or type(decoded) ~= "table" then return {} end
    return decoded :: automation_types.Map
end

local function schema_or_nil(raw: any): any?
    local schema = decode_json_table(raw)
    if next(schema) == nil then return nil end
    return schema
end

local function explicit_trigger(entry: any): automation_types.Map
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

local function trigger_from_source(desc: automation_types.Map): automation_types.Map?
    local surface = tostring(desc.surface or "")
    local kind = surface == "events" and "event" or (surface == "collection" and "collection_poll" or "")
    if kind == "" then return nil end
    local source: automation_types.Map = {
        port = desc.id,
        binding = desc.binding,
        surface = desc.surface,
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

-- list_triggers: explicit v2 trigger declarations plus compatibility
-- descriptors derived from existing source ports. Source ports remain the runtime
-- lowering; this catalog is inert and carries the v2 grammar shape for bindings.
function M.list_triggers(opts: automation_types.Map?): ({automation_types.Map}?, any)
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return nil, "registry module unavailable" end
    opts = type(opts) == "table" and opts or {}
    local component_impl = ""
    local component_id = trim((opts :: automation_types.Map).component_id)
    if component_id ~= "" then
        local component = mod("component") :: automation_types.ComponentModule?
        if not component then return nil, "component unavailable" end
        local rows, component_err = component.query({
            component_ids = { component_id },
            include = { meta = false, access = true },
            access_mask = component.ACCESS.READ,
            limit = 1,
        })
        if component_err then return nil, component_err end
        if not rows or not rows[1] then return nil, "component not found: " .. component_id end
        component_impl = trim((rows[1] :: automation_types.ComponentRow).impl_id)
        if component_impl == "" then return nil, "component implementation unavailable" end
    end
    local out: { automation_types.Map } = {}

    local entries, err = registry.find({ [".kind"] = "registry.entry", ["meta.type"] = TRIGGER_META_TYPE })
    if err then return nil, err end
    for _, entry in ipairs(entries or {}) do
        local point = explicit_trigger(entry)
        local source = type(point.source) == "table" and (point.source :: automation_types.Map) or {}
        if component_impl == "" or point.kind == "schedule" or trim(source.binding) == component_impl then
            out[#out + 1] = point
        end
    end

    local sources, serr = M.list_sources()
    if serr then return nil, serr end
    for _, desc in ipairs(sources or {}) do
        local d = desc :: automation_types.Map
        local point = (component_impl == "" or trim(d.binding) == component_impl) and trigger_from_source(d) or nil
        if point then out[#out + 1] = point end
    end

    table.sort(out, function(a: automation_types.Map, b: automation_types.Map) return tostring(a.title or "") < tostring(b.title or "") end)
    return out, nil
end

local function runnable_from_sink(desc: automation_types.Map): automation_types.Map
    return {
        id = desc.id,
        kind = "sink",
        title = desc.title,
        portable_key = desc.id,
        flow_ref = { kind = "sink", id = desc.id },
        input_schema = desc.input_schema,
        output_schema = desc.output_schema,
        invoke = {
            adapter = "kickside.automation:dispatch_to_sink",
            binding = desc.binding,
            port = desc.id,
        },
        authority_scopes = {},
        allowed_contexts = { "binding", "workflow_call" },
    }
end

-- Workflow runnables deliberately come from the workflows runnable catalog, which
-- is also the broad Run Workflow agent face and the automation-destination picker.
-- The old component/registry scan drifted from that published+runnable lifecycle
-- (and from component-first/thread projection migrations), yielding a second,
-- inconsistent definition of what could be bound.
local function workflow_runnables(_registry: automation_types.RegistryModule?): ({ automation_types.Map }?, any)
    local workflow_catalog = mod("workflow_published_catalog")
    if not workflow_catalog or type((workflow_catalog :: any).catalog) ~= "function" then return {}, nil end
    local descriptors, err = (workflow_catalog :: any).catalog()
    if err then return nil, err end
    local out: { automation_types.Map } = {}
    for _, raw in ipairs(descriptors or {}) do
        local descriptor = raw :: automation_types.Map
        local ref = type(descriptor.workflow_ref) == "table" and (descriptor.workflow_ref :: automation_types.Map) or {}
        local id = tostring(ref.id or "")
        if id ~= "" then
            local flow_ref = copy_map(ref)
            local published_interface: automation_types.Map = {
                input_schema = descriptor.input_schema,
                output_schema = descriptor.output_schema,
                error_schema = descriptor.error_schema,
            }
            out[#out + 1] = {
                id = id,
                kind = "workflow_definition",
                title = (type(descriptor.title) == "string" and descriptor.title ~= "" and descriptor.title) or id,
                portable_key = ref.portable_key,
                flow_ref = flow_ref,
                input_schema = descriptor.input_schema,
                output_schema = descriptor.output_schema,
                interface = published_interface,
                published_version = tonumber(ref.version),
                invoke = {
                    adapter = "userspace.dataflow",
                    modes = { "async", "sync" },
                    default_mode = "async",
                },
                authority_scopes = {},
                allowed_contexts = { "binding", "api", "workflow_call", "agent_tool" },
            }
        end
    end
    return out, nil
end

-- Destinations are closed and executable: workflow definitions and declared sink
-- ports. Generic runnable descriptors are intentionally excluded because their
-- arbitrary invoke metadata has no canonical Dataflow lowering.
function M.list_destinations(): ({automation_types.Map}?, any)
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return nil, "registry module unavailable" end
    local out: { automation_types.Map } = {}

    local sinks, serr = M.list_sinks()
    if serr then return nil, serr end
    for _, desc in ipairs(sinks or {}) do out[#out + 1] = runnable_from_sink(desc :: automation_types.Map) end

    local workflows, werr = workflow_runnables(registry)
    if werr then return nil, werr end
    for _, desc in ipairs(workflows or {}) do out[#out + 1] = desc end

    table.sort(out, function(a: automation_types.Map, b: automation_types.Map) return tostring(a.title or "") < tostring(b.title or "") end)
    return out, nil
end

-- ─── Binding artifact lifecycle (workflows v2 M3) ─────────────────────────
--
-- A binding is the stored join: trigger x runnable x mapping. The row is
-- the editable spec; lowerings are derived machine state keyed by binding_id.
-- Legacy `_trigger` remains untouched for old installs.
local AUTOMATION_BINDING_TABLE = "automation_bindings"
local AUTOMATION_BINDING_KIND = "kickside.automation:automation_binding_kind"
local AUTOMATION_BINDING_META_TYPE = "kickside.automation.binding"

M.AUTOMATION_BINDING_KIND = AUTOMATION_BINDING_KIND
M.AUTOMATION_BINDING_META_TYPE = AUTOMATION_BINDING_META_TYPE

local function now_rfc3339(): string
    local time_mod = mod("time")
    local n = (time_mod :: any).now()
    if type(n) == "table" and type((n :: any).utc) == "function" then n = (n :: any):utc() end
    return tostring((n :: any):format((time_mod :: any).RFC3339))
end

local function actor_id_or_nil(): string?
    local sec = mod("security")
    local actor = sec and (sec :: any).actor()
    if actor and type((actor :: any).id) == "function" then
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

local function decode_json_map(raw: any): automation_types.Map
    if type(raw) == "table" then return raw :: automation_types.Map end
    if type(raw) ~= "string" or raw == "" then return {} end
    local out, err = json.decode(raw)
    if err or type(out) ~= "table" then return {} end
    return out :: automation_types.Map
end

-- Old binding rows may name a workflow through the retired dynamic-backend
-- namespace. The workflow component id was always the suffix, so reads expose
-- the one current workflow_definition ref without rewriting historical rows.
local function stored_flow_ref(raw: any): automation_types.Map
    local ref = decode_json_map(raw)
    local workflow_ref = mod("workflow_ref")
    if not workflow_ref or type((workflow_ref :: any).normalize_legacy_workflow_ref) ~= "function" then return ref end
    local canonical = (workflow_ref :: any).normalize_legacy_workflow_ref(ref)
    return type(canonical) == "table" and canonical or ref
end

local function is_enabled_value(raw: any): boolean
    return raw == true or raw == 1 or raw == "1" or raw == "true"
end

local function binding_db(): (any?, string?)
    local sql_mod = mod("sql")
    if not sql_mod then return nil, "sql module unavailable" end
    local db, err = (sql_mod :: any).get(automation_types.db_id())
    if err or not db then return nil, "automation bindings db unavailable: " .. tostring(err) end
    return db, nil
end

local function row_to_binding(row: any): automation_types.Map
    local r = type(row) == "table" and (row :: automation_types.Map) or {}
    return {
        binding_id = tostring(r.binding_id or ""),
        portable_key = tostring(r.portable_key or ""),
        title = tostring(r.title or ""),
        enabled = is_enabled_value(r.enabled),
        trigger_id = tostring(r.trigger_id or ""),
        trigger_config = decode_json_map(r.trigger_config),
        flow_ref = stored_flow_ref(r.flow_ref),
        mapping_spec = decode_json_map(r.mapping_spec),
        guard_expr = type(r.guard_expr) == "string" and r.guard_expr or nil,
        execution_policy = decode_json_map(r.execution_policy),
        authority_scope = decode_json_map(r.authority_scope),
        legacy_trigger_id = type(r.legacy_trigger_id) == "string" and r.legacy_trigger_id or nil,
        lowering_state = decode_json_map(r.lowering_state),
        created_by = r.created_by,
        updated_by = r.updated_by,
        created_at = r.created_at,
        updated_at = r.updated_at,
    }
end

local function get_binding_row(binding_id: string): (automation_types.Map?, string?)
    local id = trim(binding_id)
    if id == "" then return nil, "binding_id is required" end
    local db, derr = binding_db()
    if not db then return nil, derr end
    local rows, qerr = (db :: any):query("SELECT * FROM " .. AUTOMATION_BINDING_TABLE .. " WHERE binding_id = $1 LIMIT 1", { id })
    if type((db :: any).release) == "function" then (db :: any):release() end
    if qerr then return nil, tostring(qerr) end
    if type(rows) ~= "table" or not (rows :: { any })[1] then return nil, nil end
    return row_to_binding((rows :: { any })[1]), nil
end

local function normalize_binding_execution_policy(raw_policy: any): (automation_types.Map?, string?)
    local policy = type(raw_policy) == "table" and copy_map(raw_policy) or {}
    local mode = trim(policy.mode)
    if mode == "" then mode = "component_owner" end
    policy.mode = mode
    if mode ~= "component_owner" then
        return nil, "execution_policy.mode must be component_owner"
    end
    return policy, nil
end

function M.get_binding(binding_id: string): (automation_types.Map?, any)
    local row, err = get_binding_row(binding_id)
    if err then return nil, err end
    if not row then return nil, "binding not found: " .. tostring(binding_id) end
    return row, nil
end

-- Workflows consumes this narrow read through kickside.automation:binding_consumers
-- when rebuilding its reverse index. The automation-owned table remains private.
function M.list_workflow_binding_consumers(_args: any?): (automation_types.Map?, any)
    local db, derr = binding_db()
    if not db then return nil, derr end
    local rows, qerr = (db :: any):query(
        "SELECT binding_id, flow_ref FROM " .. AUTOMATION_BINDING_TABLE .. " ORDER BY binding_id ASC", {})
    if type((db :: any).release) == "function" then (db :: any):release() end
    if qerr then return nil, tostring(qerr) end

    local bindings: { automation_types.Map } = {}
    for _, row in ipairs(type(rows) == "table" and rows or {}) do
        local r = row :: automation_types.Map
        local binding_id = trim(r.binding_id)
        if binding_id ~= "" then
            bindings[#bindings + 1] = {
                binding_id = binding_id,
                flow_ref = stored_flow_ref(r.flow_ref),
            }
        end
    end
    table.sort(bindings, function(a: automation_types.Map, b: automation_types.Map): boolean
        return tostring(a.binding_id) < tostring(b.binding_id)
    end)
    return { bindings = bindings }, nil
end

local find_trigger: ((string) -> (automation_types.Map?, string?))?

-- The cron implementation evaluates expressions in UTC. Keep that limitation
-- explicit in the declared periodic trigger contract instead of accepting an
-- IANA zone that the underlying scheduler would silently ignore. Interval
-- schedules still carry tz in their context, which makes bindings portable and
-- leaves room for cron timezone support to be added at the scheduler seam.
local function normalize_periodic_trigger_config(raw: any): (automation_types.Map?, string?)
    if type(raw) ~= "table" then return nil, "periodic trigger_config must be an object" end
    local config = raw :: automation_types.Map
    local schedule = type(config.schedule) == "table" and (config.schedule :: automation_types.Map) or nil
    if not schedule then return nil, "periodic trigger_config.schedule is required" end
    local schedule_type = trim(schedule.type)
    if schedule_type ~= "interval" and schedule_type ~= "cron" then
        return nil, "periodic schedule.type must be interval or cron"
    end
    local expression = trim(schedule.expression)
    if expression == "" then return nil, "periodic schedule.expression is required" end
    if schedule_type == "interval" then
        local time_mod = mod("time")
        if not time_mod or type((time_mod :: any).parse_duration) ~= "function" then
            return nil, "periodic interval validation is unavailable"
        end
        local _, duration_err = (time_mod :: any).parse_duration(expression)
        if duration_err then return nil, "periodic interval expression is invalid: " .. tostring(duration_err) end
    else
        local _, fields = expression:gsub("%S+", "")
        if fields ~= 5 then
            return nil, "periodic cron expression must have exactly 5 fields: minute hour day month weekday"
        end
    end
    local tz = trim(schedule.tz)
    if tz ~= "UTC" then
        return nil, "periodic schedule.tz must be UTC; cron expressions are evaluated in UTC"
    end
    return {
        schedule = {
            type = schedule_type,
            expression = expression,
            tz = tz,
        },
    }, nil
end

M.SCHEDULE_TRIGGER = SCHEDULE_TRIGGER
M._normalize_periodic_trigger_config = normalize_periodic_trigger_config

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
        for k, _ in pairs(value :: automation_types.Map) do
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
    if actual == "array" and type(value) == "table" and next(value :: automation_types.Map) == nil then
        if schema_type_declares(schema_type, "object") then return "object" end
        if schema_type_declares(schema_type, "array") then return "array" end
    end
    return actual
end

local validate_schema_value: (any, any, string) -> string?

local function validate_schema_object(schema: automation_types.Map, value: any, path: string): string?
    if type(value) ~= "table" then return path .. " must be an object" end
    if value_kind(value) ~= "object" and next(value :: automation_types.Map) ~= nil then return path .. " must be an object" end
    local props = type(schema.properties) == "table" and (schema.properties :: automation_types.Map) or {}
    if type(schema.required) == "table" then
        for _, raw_key in ipairs(schema.required :: { any }) do
            local key = tostring(raw_key or "")
            if key == "" then return "invalid schema: required entries must be strings" end
            if (value :: automation_types.Map)[key] == nil then return path .. "." .. key .. " is required" end
        end
    end
    for key, child_schema in pairs(props) do
        local child = (value :: automation_types.Map)[key]
        if child ~= nil then
            local err = validate_schema_value(child_schema, child, path .. "." .. tostring(key))
            if err then return err end
        end
    end
    if schema.additionalProperties == false then
        for key, _ in pairs(value :: automation_types.Map) do
            if props[key] == nil then return path .. "." .. tostring(key) .. " is not accepted" end
        end
    end
    return nil
end

validate_schema_value = function(schema: any, value: any, path: string): string?
    if type(schema) ~= "table" then return nil end
    local s = schema :: automation_types.Map
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
    local s = type(schema) == "table" and (schema :: automation_types.Map) or {}
    local typ = s.type
    if type(typ) == "table" then typ = (typ :: { any })[1] end
    if typ == "object" or type(s.properties) == "table" then
        local out: automation_types.Map = {}
        if type(s.properties) == "table" then
            for key, child in pairs(s.properties :: automation_types.Map) do out[tostring(key)] = schema_sample(child) end
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

local function workflow_start_schema_for_ref(ref: automation_types.Map): (automation_types.Map?, string?)
    if trim(ref.kind) ~= "workflow_definition" then return nil, nil end
    local workflow_id = trim(ref.id)
    if workflow_id == "" then workflow_id = trim(ref.workflow_id) end
    if workflow_id == "" then return nil, "flow_ref.id is required for workflow_definition" end
    local db, derr = binding_db()
    if not db then return nil, derr end
    local workflow_store = mod("workflow_store")
    if not workflow_store or type((workflow_store :: any).version_interface) ~= "function" then
        if type((db :: any).release) == "function" then (db :: any):release() end
        return nil, "workflow interface store unavailable"
    end
    local iface, iface_err = (workflow_store :: any).version_interface(db, workflow_id, tonumber(ref.version))
    if iface_err then
        if type((db :: any).release) == "function" then (db :: any):release() end
        return nil, tostring(iface_err)
    end
    if type(iface) == "table" and type((iface :: automation_types.Map).input_schema) == "table" then
        if type((db :: any).release) == "function" then (db :: any):release() end
        return (iface :: automation_types.Map).input_schema, nil
    end
    local params: { any } = { workflow_id }
    local version_clause = ""
    if tonumber(ref.version) then
        version_clause = " AND version = $2"
        params[#params + 1] = tonumber(ref.version)
    end
    local rows, qerr = (db :: any):query([[
        SELECT version, document
          FROM workflow_versions
         WHERE workflow_id = $1
    ]] .. version_clause .. [[
         ORDER BY version DESC
         LIMIT 1
    ]], params)
    if type((db :: any).release) == "function" then (db :: any):release() end
    if qerr then return nil, tostring(qerr) end
    if type(rows) ~= "table" or not (rows :: { any })[1] then
        return nil, "workflow_definition runnable must reference a published workflow: " .. workflow_id
    end
    local doc = decode_json_table(((rows :: { any })[1] :: automation_types.Map).document)
    if type((workflow_store :: any).start_input_schema_from_document) ~= "function" then
        return nil, "workflow interface store unavailable"
    end
    return (workflow_store :: any).start_input_schema_from_document(doc), nil
end

local function apply_mapping_sample(mapping_spec: any, trigger_id: string): (any?, string?)
    local point: automation_types.Map? = nil
    if find_trigger then point = select(1, find_trigger(trigger_id)) end
    if not point then return nil, "trigger not found: " .. trigger_id end
    local input = schema_sample((point :: automation_types.Map).context_schema)
    if type(input) ~= "table" then input = {} end
    ; (input :: automation_types.Map).trigger_id = trigger_id
    ; (input :: automation_types.Map).binding_id = "binding-sample"
    ; (input :: automation_types.Map).occurred_at = "2026-01-01T00:00:00Z"
    ; (input :: automation_types.Map).event_type = trim((point :: automation_types.Map).kind)

    local mapping = type(mapping_spec) == "table" and (mapping_spec :: automation_types.Map) or {}
    if next(mapping) == nil then return input, nil end
    local mode = trim(mapping.mode)
    if mode == "" then mode = "expr" end
    if mode ~= "expr" and mode ~= "expr_generated" then return nil, "binding mapping mode unsupported: " .. mode end
    local source = trim(mapping.expr)
    if source == "" then return nil, "binding mapping expr is required" end
    local expr_mod = mod("expr")
    if not expr_mod or type((expr_mod :: any).eval) ~= "function" then return nil, "expr module unavailable" end
    local out, err = (expr_mod :: any).eval(source, { input = input })
    if err then return nil, "binding mapping expr: " .. tostring(err) end
    return type(out) == "table" and out or { value = out }, nil
end

local function validate_workflow_binding_mapping(spec: automation_types.Map): string?
    local flow_ref = type(spec.flow_ref) == "table" and (spec.flow_ref :: automation_types.Map) or {}
    if trim(flow_ref.kind) ~= "workflow_definition" then return nil end
    local start_schema, serr = workflow_start_schema_for_ref(flow_ref)
    if serr or not start_schema then return serr or "workflow Start schema unavailable" end
    local mapped, merr = apply_mapping_sample(spec.mapping_spec, trim(spec.trigger_id))
    if merr then return merr end
    local verr = validate_schema_value(start_schema, mapped, "mapping output")
    if verr then return "binding mapping output does not match workflow Start schema: " .. verr end
    return nil
end

local function validate_binding(input: any): (automation_types.Map?, string?)
    if type(input) ~= "table" then return nil, "binding spec must be a table" end
    local raw = input :: automation_types.Map
    local portable_key = trim(raw.portable_key)
    if portable_key == "" then return nil, "portable_key is required" end
    local title = trim(raw.title)
    if title == "" then return nil, "title is required" end
    local trigger_id = trim(raw.trigger_id)
    if trigger_id == "" then return nil, "trigger_id is required" end
    local trigger_config = type(raw.trigger_config) == "table" and raw.trigger_config or {}
    if trigger_id == SCHEDULE_TRIGGER then
        local normalized, schedule_err = normalize_periodic_trigger_config(trigger_config)
        if schedule_err or not normalized then return nil, schedule_err or "invalid periodic trigger config" end
        trigger_config = normalized
    end
    local flow_ref = type(raw.flow_ref) == "table" and copy_map(raw.flow_ref) or {}
    local flow_kind = trim(flow_ref.kind)
    if flow_kind == "" then return nil, "flow_ref.kind is required" end
    if flow_kind == "workflow_definition" then
        local workflow_id = trim(flow_ref.id)
        if workflow_id == "" then return nil, "flow_ref.id is required for workflow_definition" end
        local workflow_ref = mod("workflow_ref")
        local was_legacy = false
        if workflow_ref and type((workflow_ref :: any).normalize_legacy_workflow_ref) == "function" then
            local _normalized: any
            _normalized, was_legacy = (workflow_ref :: any).normalize_legacy_workflow_ref(flow_ref)
        end
        if was_legacy then
            return nil, "flow_ref.id uses retired partial workflow vocabulary; use the workflow component id"
        end
        flow_ref.id = workflow_id
    elseif flow_kind == "behavior" then
        if trim(flow_ref.behavior_id) == "" then return nil, "flow_ref.behavior_id is required for behavior" end
        local instance_id = trim(flow_ref.component_instance_id or flow_ref.component_instance)
        if instance_id == "" then return nil, "flow_ref.component_instance_id is required for behavior" end
        flow_ref.component_instance_id = instance_id
        flow_ref.component_instance = nil
    elseif flow_kind == "sink" then
        if trim(flow_ref.id) == "" then return nil, "flow_ref.id is required for sink" end
    else
        return nil, "flow_ref.kind must be workflow_definition, behavior, or sink"
    end
    local execution_policy, policy_err = normalize_binding_execution_policy(raw.execution_policy)
    if policy_err or not execution_policy then return nil, policy_err or "invalid execution_policy" end
    return {
        portable_key = portable_key,
        title = title,
        enabled = raw.enabled == true,
        trigger_id = trigger_id,
        trigger_config = trigger_config,
        flow_ref = flow_ref,
        mapping_spec = type(raw.mapping_spec) == "table" and raw.mapping_spec or {},
        guard_expr = type(raw.guard_expr) == "string" and trim(raw.guard_expr) ~= "" and trim(raw.guard_expr) or nil,
        execution_policy = execution_policy,
        authority_scope = type(raw.authority_scope) == "table" and raw.authority_scope or {},
        legacy_trigger_id = type(raw.legacy_trigger_id) == "string" and trim(raw.legacy_trigger_id) ~= "" and trim(raw.legacy_trigger_id) or nil,
    }, nil
end

local function register_binding_component(binding_id: string, title: string, parent_id: any, identity: any, flow_ref: automation_types.Map): string?
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return "component unavailable" end
    local svc, svc_err = component.get_service()
    if not svc then return "component service: " .. tostring(svc_err) end
    local meta: automation_types.Map = {
        title = title,
        icon = "tabler:plug-connected",
        class = "automation_binding",
        flow_ref = copy_map(flow_ref),
        enabled = false,
        status = "paused",
    }
    local req: automation_types.Map = {
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

local function binding_id_new(): string
    local uuid_mod = mod("uuid")
    if uuid_mod and type((uuid_mod :: any).v7) == "function" then return tostring((uuid_mod :: any).v7()) end
    return tostring(uuid.v7())
end

function M.create_binding(input: any, parent_id: string?): (automation_types.Map?, any)
    local spec, verr = validate_binding(input)
    if not spec then return nil, verr end
    local schema_err = validate_workflow_binding_mapping(spec)
    if schema_err then return nil, schema_err end
    local binding_id = trim((type(input) == "table" and (input :: automation_types.Map).binding_id) or "")
    if binding_id == "" then binding_id = binding_id_new() end
    local identity, identity_err = capture_execution_identity()
    if identity_err or not identity then return nil, identity_err or "could not capture binding execution identity" end
    local cerr = register_binding_component(binding_id, tostring(spec.title), parent_id, identity, spec.flow_ref)
    if cerr then return nil, cerr end

    local created_at = now_rfc3339()
    local actor_id = actor_id_or_nil()
    local lowering_state: automation_types.Map = {}
    local db, derr = binding_db()
    if not db then return nil, derr end
    local _, ierr = (db :: any):execute([[
        INSERT INTO automation_bindings
            (binding_id, portable_key, title, enabled, trigger_id, trigger_config,
             flow_ref, mapping_spec, guard_expr, execution_policy, authority_scope,
             legacy_trigger_id, lowering_state, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
    ]], {
        binding_id,
        spec.portable_key,
        spec.title,
        spec.enabled == true,
        spec.trigger_id,
        encode_json(spec.trigger_config),
        encode_json(spec.flow_ref),
        encode_json(spec.mapping_spec),
        spec.guard_expr,
        encode_json(spec.execution_policy),
        encode_json(spec.authority_scope),
        spec.legacy_trigger_id,
        encode_json(lowering_state),
        actor_id,
        actor_id,
        created_at,
        created_at,
    })
    if ierr then
        if type((db :: any).release) == "function" then (db :: any):release() end
        return nil, tostring(ierr)
    end
    local workflow_store = mod("workflow_store")
    if not workflow_store or type((workflow_store :: any).replace_automation_binding_consumer) ~= "function" then
        if type((db :: any).release) == "function" then (db :: any):release() end
        return nil, "workflow consumer store unavailable"
    end
    local consumer_err = (workflow_store :: any).replace_automation_binding_consumer(db, binding_id, spec.flow_ref)
    if type((db :: any).release) == "function" then (db :: any):release() end
    if consumer_err then return nil, "workflow consumer projection failed: " .. tostring(consumer_err) end
    local row = {
        binding_id = binding_id,
        portable_key = spec.portable_key,
        title = spec.title,
        enabled = false,
        trigger_id = spec.trigger_id,
        trigger_config = spec.trigger_config,
        flow_ref = spec.flow_ref,
        mapping_spec = spec.mapping_spec,
        guard_expr = spec.guard_expr,
        execution_policy = spec.execution_policy,
        authority_scope = spec.authority_scope,
        legacy_trigger_id = spec.legacy_trigger_id,
        lowering_state = lowering_state,
        created_by = actor_id,
        updated_by = actor_id,
        created_at = created_at,
        updated_at = created_at,
    }
    if spec.enabled == true then
        return M.enable_binding(binding_id)
    end
    return row, nil
end

local function persist_binding_enabled(binding_id: string, enabled: boolean, lowering_state: any): (automation_types.Map?, any)
    local db, derr = binding_db()
    if not db then return nil, derr end
    local updated_at = now_rfc3339()
    local _, uerr = (db :: any):execute(
        "UPDATE " .. AUTOMATION_BINDING_TABLE .. " SET enabled = $1, lowering_state = $2, updated_at = $3 WHERE binding_id = $4",
        { enabled, encode_json(lowering_state), updated_at, binding_id })
    if type((db :: any).release) == "function" then (db :: any):release() end
    if uerr then return nil, tostring(uerr) end
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end
    local ok, meta_err = (component :: any).set_meta(binding_id, {
        enabled = enabled,
        status = enabled and "idle" or "paused",
    })
    if not ok then return nil, "binding public state update failed: " .. tostring(meta_err) end
    local row, rerr = get_binding_row(binding_id)
    if rerr then return nil, rerr end
    if not row then return nil, "binding not found: " .. binding_id end
    return row, nil
end

local function lowering_is_attached(state: any): boolean
    return type(state) == "table" and trim((state :: automation_types.Map).mode) ~= ""
end

-- The cron row a lowering names, for the machines that are backed by one. Other
-- machines carry no schedule and are not asked about.
local function lowering_schedule_id(state: any): string
    local s = type(state) == "table" and (state :: automation_types.Map) or {}
    local registration = type(s.registration) == "table" and (s.registration :: automation_types.Map) or {}
    local schedule_id = trim(registration.schedule_id)
    if schedule_id ~= "" then return schedule_id end
    return trim(registration.poll_schedule_id)
end

-- Test seam over the cron read behind lowering_stands.
M._schedule_exists = function(schedule_id: string): (boolean, any)
    return (mod("automation_schedule") :: any).action_schedule_exists(schedule_id)
end

-- A lowering stands only while the machine it names still exists. Retention
-- purges a failed cron row after its window, and enabling against a registration
-- that names a row which is gone updates nothing: the binding reads as on and
-- never fires. The binding still holds its schedule spec, so the answer is to
-- build the machine again.
--
-- Only a proven absence re-lowers. A read that could not answer leaves the
-- lowering alone, because tearing down live work on an unreadable database would
-- be worse than the state it was meant to repair.
local function lowering_stands(state: any): boolean
    local schedule_id = lowering_schedule_id(state)
    if schedule_id == "" then return true end
    local exists, err = M._schedule_exists(schedule_id)
    if err then return true end
    return exists == true
end

function M.enable_binding(binding_id: string): (automation_types.Map?, any)
    local binding, err = get_binding_row(binding_id)
    if err then return nil, err end
    if not binding then return nil, "binding not found: " .. tostring(binding_id) end
    local _, policy_err = normalize_binding_execution_policy(binding.execution_policy)
    if policy_err then return nil, policy_err end
    local lowering_state = binding.lowering_state
    if not lowering_is_attached(lowering_state) or not lowering_stands(lowering_state) then
        local lowered, lerr = M._attach_binding_lowering(binding)
        if lerr or not lowered then return nil, lerr or "binding lowering failed" end
        lowering_state = lowered
    end
    return persist_binding_enabled(binding.binding_id, true, lowering_state)
end

function M.disable_binding(binding_id: string): (automation_types.Map?, any)
    local binding, err = get_binding_row(binding_id)
    if err then return nil, err end
    if not binding then return nil, "binding not found: " .. tostring(binding_id) end
    local lowering_state = binding.lowering_state
    if lowering_is_attached(lowering_state) then
        local _, derr = M._detach_binding_lowering(lowering_state)
        if derr then return nil, derr end
    end
    return persist_binding_enabled(binding.binding_id, false, {})
end

local function binding_component_id(): (string?, any)
    local ctx_mod = mod("ctx")
    local id = trim(ctx_mod and type((ctx_mod :: any).get) == "function" and (ctx_mod :: any).get("component_id") or nil)
    if id == "" then return nil, "binding component_id is required" end
    return id, nil
end

-- Binding artifact shells use the same pausable action contract as legacy
-- automation kinds. Their durable enabled state and cron ownership live in the
-- binding row, so controls intentionally delegate to the existing lowering
-- lifecycle rather than writing component metadata.
function M.pause_binding(_args: any): (automation_types.Map?, any)
    local id, id_err = binding_component_id()
    if id_err or not id then return nil, id_err end
    local binding, err = M.disable_binding(id)
    if err or not binding then return nil, err or "binding pause failed" end
    return { success = true, id = id, enabled = binding.enabled }, nil
end

function M.resume_binding(_args: any): (automation_types.Map?, any)
    local id, id_err = binding_component_id()
    if id_err or not id then return nil, id_err end
    local binding, err = M.enable_binding(id)
    if err or not binding then return nil, err or "binding resume failed" end
    return { success = true, id = id, enabled = binding.enabled }, nil
end

-- Binding configuration is immutable after creation except for its lifecycle
-- switch. Keeping PUT on the generic reconfigure surface lets callers use the
-- same control plane while preventing an accidental trigger/runnable rewrite.
function M.reconfigure_binding(args: any): (automation_types.Map?, any)
    local input = type(args) == "table" and (args :: automation_types.Map) or nil
    if not input or type(input.enabled) ~= "boolean" then
        return nil, "binding reconfigure requires enabled boolean"
    end
    for key, _ in pairs(input) do
        if key ~= "enabled" then return nil, "binding reconfigure only supports enabled" end
    end
    if input.enabled then return M.resume_binding({}) end
    return M.pause_binding({})
end

local function binding_public_state(binding: automation_types.Map): automation_types.Map
    local enabled = binding.enabled == true
    return { enabled = enabled, status = enabled and "idle" or "paused" }
end

local function binding_config(binding: automation_types.Map): automation_types.Map
    return {
        portable_key = binding.portable_key,
        title = binding.title,
        enabled = binding.enabled,
        trigger_id = binding.trigger_id,
        trigger_config = binding.trigger_config,
        flow_ref = binding.flow_ref,
        mapping_spec = binding.mapping_spec,
        guard_expr = binding.guard_expr,
        execution_policy = binding.execution_policy,
        authority_scope = binding.authority_scope,
        legacy_trigger_id = binding.legacy_trigger_id,
    }
end

-- delete_binding_data removes the binding's durable side data. It is deliberately
-- only reached from the binding kind's deletable contract: component teardown
-- invokes that contract before unregistering the component, so a failed schedule
-- detach retains both the binding row and component for a safe retry.
local function delete_binding_data(id: string): (automation_types.Map?, any)
    local existing, read_err = get_binding_row(id)
    if read_err then return nil, read_err end
    if existing then
        local _, derr = M.disable_binding(id)
        if derr then return nil, derr end
    end
    local db, dberr = binding_db()
    if not db then return nil, dberr end
    local workflow_store = mod("workflow_store")
    if not workflow_store or type((workflow_store :: any).replace_automation_binding_consumer) ~= "function" then
        if type((db :: any).release) == "function" then (db :: any):release() end
        return nil, "workflow consumer store unavailable"
    end
    local consumer_err = (workflow_store :: any).replace_automation_binding_consumer(db, id, nil)
    if consumer_err then
        if type((db :: any).release) == "function" then (db :: any):release() end
        return nil, "workflow consumer projection failed: " .. tostring(consumer_err)
    end
    local _, xerr = (db :: any):execute("DELETE FROM " .. AUTOMATION_BINDING_TABLE .. " WHERE binding_id = $1", { id })
    if type((db :: any).release) == "function" then (db :: any):release() end
    if xerr then return nil, tostring(xerr) end

    return { success = true, binding_id = id }, nil
end

-- delete_binding is both the public binding-delete entry point and the v2
-- binding kind's deletable handler. An explicit id enters the standard component
-- teardown path. The component service invokes this same function with the
-- component context and no id; only that lifecycle invocation removes the
-- binding row and its lowered resources. Keeping those roles on one path means
-- direct deletion, HTTP deletion, and lifecycle reaping have identical cleanup.
function M.delete_binding(binding_id: any): (automation_types.Map?, any)
    local id = trim(binding_id)
    if id == "" then
        local ctx_mod = mod("ctx")
        id = trim(ctx_mod and type((ctx_mod :: any).get) == "function" and (ctx_mod :: any).get("component_id") or nil)
        if id == "" then return nil, "binding component_id is required" end
        return delete_binding_data(id)
    end

    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end
    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local deleted, cerr = svc:delete({ component_id = id })
    if cerr or not deleted or (type(deleted) == "table" and (deleted :: automation_types.Map).success == false) then
        return nil, "delete binding component: " .. tostring(cerr or
            (type(deleted) == "table" and (deleted :: automation_types.Map).error) or "no result")
    end
    return { success = true, binding_id = id }, nil
end

function M.list_bindings(opts: any): ({ automation_types.Map }?, any)
    local options = type(opts) == "table" and (opts :: automation_types.Map) or {}
    local db, derr = binding_db()
    if not db then return nil, derr end
    local rows: any
    local qerr: any
    if trim(options.legacy_trigger_id) ~= "" then
        rows, qerr = (db :: any):query("SELECT * FROM " .. AUTOMATION_BINDING_TABLE .. " WHERE legacy_trigger_id = $1 ORDER BY updated_at DESC", { trim(options.legacy_trigger_id) })
    elseif trim(options.trigger_id) ~= "" then
        rows, qerr = (db :: any):query("SELECT * FROM " .. AUTOMATION_BINDING_TABLE .. " WHERE trigger_id = $1 ORDER BY updated_at DESC", { trim(options.trigger_id) })
    else
        rows, qerr = (db :: any):query("SELECT * FROM " .. AUTOMATION_BINDING_TABLE .. " ORDER BY updated_at DESC", {})
    end
    if type((db :: any).release) == "function" then (db :: any):release() end
    if qerr then return nil, tostring(qerr) end
    local out: { automation_types.Map } = {}
    for _, row in ipairs(type(rows) == "table" and rows or {}) do
        local binding = row_to_binding(row)
        if options.enabled == nil or binding.enabled == options.enabled then out[#out + 1] = binding end
    end
    return out, nil
end

local function persist_binding_lowering_state(binding_id: string, lowering_state: any): (automation_types.Map?, any)
    local db, derr = binding_db()
    if not db then return nil, derr end
    local updated_at = now_rfc3339()
    local _, uerr = (db :: any):execute(
        "UPDATE " .. AUTOMATION_BINDING_TABLE .. " SET lowering_state = $1, updated_at = $2 WHERE binding_id = $3",
        { encode_json(lowering_state), updated_at, binding_id })
    if type((db :: any).release) == "function" then (db :: any):release() end
    if uerr then return nil, tostring(uerr) end
    local row, rerr = get_binding_row(binding_id)
    if rerr then return nil, rerr end
    if not row then return nil, "binding not found: " .. binding_id end
    return row, nil
end

find_trigger = function(trigger_id: string): (automation_types.Map?, string?)
    local points, err = M.list_triggers()
    if err then return nil, tostring(err) end
    for _, point in ipairs(points or {}) do
        if trim((point :: automation_types.Map).id) == trigger_id then return point :: automation_types.Map, nil end
    end
    return nil, "trigger not found: " .. trigger_id
end

local function trigger_spec_for_binding(binding: automation_types.Map): (automation_types.Map?, string?, string?)
    local point, perr = find_trigger(trim(binding.trigger_id))
    if perr or not point then return nil, perr, nil end
    local kind = trim(point.kind)
    local config = type(binding.trigger_config) == "table" and (binding.trigger_config :: automation_types.Map) or {}
    if kind == "ui_action" then
        return { v = 1, mode = "ui_action", selector = config }, nil, kind
    end
    if kind == "schedule" then
        local schedule = type(config.schedule) == "table" and (config.schedule :: automation_types.Map) or config
        return { v = 1, schedule = schedule, config = {} }, nil, kind
    end
    local source = type(point.source) == "table" and (point.source :: automation_types.Map) or {}
    local port = trim(source.port)
    if port == "" then port = trim(source.source) end
    if port == "" and (kind == "event" or kind == "webhook" or kind == "collection_poll") then
        port = trim(point.id)
    end
    if kind == "event" or kind == "webhook" or kind == "lifecycle" then
        local spec: automation_types.Map = {
            v = 1,
            source = port,
            config = config,
            backfill = config.backfill or "none",
        }
        return spec, nil, kind
    end
    if kind == "collection_poll" then
        local spec: automation_types.Map = {
            v = 1,
            source = port,
            config = config,
            backfill = config.backfill or "all",
            schedule = type(config.schedule) == "table" and config.schedule or nil,
        }
        return spec, nil, kind
    end
    return nil, "unsupported trigger kind for lowering: " .. kind, nil
end

local function open_trigger_service(): (any?, string?)
    local contract_mod = mod("contract")
    local def, derr = (contract_mod :: any).get("kickside.trigger:service")
    if derr or not def then return nil, "trigger service contract unavailable: " .. tostring(derr) end
    local inst, oerr = (def :: any):open()
    if oerr or not inst then return nil, "trigger service open: " .. tostring(oerr) end
    return inst, nil
end

M._attach_binding_lowering = function(binding: any): (any?, any)
    local b = type(binding) == "table" and (binding :: automation_types.Map) or {}
    local spec, serr, trigger_kind = trigger_spec_for_binding(b)
    if serr or not spec then return nil, serr end
    if spec.mode == "ui_action" then
        return {
            v = 1,
            mode = "ui_action",
            phase = "live",
            selector = spec.selector or {},
            trigger_id = b.trigger_id,
        }, nil
    end

    local service, oerr = open_trigger_service()
    if oerr or not service then return nil, oerr end
    local rollback: { automation_types.Map } = {}
    local recorder = {
        rollback = function(target_id: string, args: automation_types.Map?)
            if trim(target_id) ~= "" then
                rollback[#rollback + 1] = { target = target_id, args = type(args) == "table" and args or {} }
            end
        end,
    }
    local handler_ref = trigger_kind == "collection_poll" and "poll" or "execute"
    local installed, ierr = (service :: any):install({
        spec = spec,
        consumer = {
            component_id = b.binding_id,
            handler_ref = handler_ref,
            consumer_kind = "automation_binding",
            description = "Binding lowering: " .. tostring(b.title or b.binding_id),
        },
        recorder = recorder,
        enabled = true,
    })
    if ierr then return nil, tostring(ierr) end
    local out = type(installed) == "table" and (installed :: automation_types.Map) or {}
    if out.success ~= true then
        local e = type(out.error) == "table" and (out.error :: automation_types.Map) or {}
        return nil, tostring(e.message or out.error or "trigger service install failed")
    end
    local trigger = type(out.trigger) == "table" and (out.trigger :: automation_types.Map) or {}
    -- The lowering installs the block the service just built onto the binding's
    -- own component, against whatever that component's context states now.
    local component_mod = mod("component") :: automation_types.ComponentModule?
    local lowering_state_before = component_mod
        and select(1, component_mod.get_context(tostring(b.binding_id or ""), component_mod.ACCESS.WRITE))
        or nil
    local _, werr = M.write_trigger_state(tostring(b.binding_id or ""), trigger,
        M.trigger_precondition(lowering_state_before))
    if werr then
        M.replay(rollback)
        return nil, "write binding trigger state: " .. tostring(werr)
    end
    return {
        v = 1,
        component_id = b.binding_id,
        mode = trim(trigger.mode),
        phase = trim(trigger.phase) ~= "" and trigger.phase or "live",
        spec = type(trigger.spec) == "table" and trigger.spec or spec,
        registration = type(trigger.registration) == "table" and trigger.registration or {},
        rollback = rollback,
        cursor = trigger.cursor,
    }, nil
end

M._detach_binding_lowering = function(lowering_state: any): (any?, any)
    local state = type(lowering_state) == "table" and (lowering_state :: automation_types.Map) or {}
    local mode = trim(state.mode)
    if mode == "" or mode == "ui_action" then return { success = true }, nil end
    local component_id = trim(state.component_id)
    if component_id == "" then return nil, "binding lowering component_id missing" end
    local service, oerr = open_trigger_service()
    if oerr or not service then return nil, oerr end
    local detached, derr = (service :: any):uninstall({ component_id = component_id })
    if derr then return nil, tostring(derr) end
    local d = type(detached) == "table" and (detached :: automation_types.Map) or {}
    if d.success == false then
        local e = type(d.error) == "table" and (d.error :: automation_types.Map) or {}
        return nil, tostring(e.message or d.error or "detach failed")
    end
    return { success = true }, nil
end

local function mapping_input_context(binding: automation_types.Map, raw_context: any, args: automation_types.Map): automation_types.Map
    local input = type(raw_context) == "table" and copy_map(raw_context) or {}
    input.trigger_id = input.trigger_id or binding.trigger_id
    input.binding_id = input.binding_id or binding.binding_id
    input.occurred_at = input.occurred_at or args.occurred_at
    input.event_type = input.event_type or args.event_type
    return input
end

local function eval_expr(source: string, env: automation_types.Map): (any?, any)
    local expr_mod = mod("expr")
    return (expr_mod :: any).eval(source, env)
end

local function apply_binding_mapping(spec: any, input: automation_types.Map): (any?, any)
    local mapping = type(spec) == "table" and (spec :: automation_types.Map) or {}
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

local function binding_guard_allows(binding: automation_types.Map, input: automation_types.Map): (boolean, any)
    local guard = trim(binding.guard_expr)
    if guard == "" then return true, nil end
    local out, err = eval_expr(guard, { input = input })
    if err then return false, "binding guard expr: " .. tostring(err) end
    return not (out == nil or out == false), nil
end

local function shadow_compare_enabled(binding: automation_types.Map): boolean
    local policy = type(binding.execution_policy) == "table" and (binding.execution_policy :: automation_types.Map) or {}
    if policy.shadow_compare == true then return true end
    local state = type(binding.lowering_state) == "table" and (binding.lowering_state :: automation_types.Map) or {}
    if trim(state.phase) == "shadow" or trim(state.mode) == "legacy_shadow" then return true end
    local migration = type(state.migration) == "table" and (state.migration :: automation_types.Map) or {}
    return trim(migration.phase) == "shadow"
end

local function record_shadow_compare(binding: automation_types.Map, launch_args: automation_types.Map, input: automation_types.Map, mapped: any, args: automation_types.Map): (automation_types.Map?, any)
    local state = type(binding.lowering_state) == "table" and copy_map(binding.lowering_state) or {}
    local migration = type(state.migration) == "table" and copy_map(state.migration) or {}
    local compare = type(migration.shadow_compare) == "table" and copy_map(migration.shadow_compare) or {}
    compare.count = (tonumber(compare.count) or 0) + 1
    compare.last = {
        observed_at = now_rfc3339(),
        dedup_key = trim(args.dedup_key) ~= "" and trim(args.dedup_key) or nil,
        trigger_id = launch_args.trigger_id,
        flow_ref = launch_args.flow_ref,
        input = input,
        mapped_input = mapped,
        would_launch = launch_args,
    }
    migration.phase = trim(migration.phase) ~= "" and migration.phase or "shadow"
    migration.shadow_compare = compare
    state.migration = migration
    if trim(state.mode) == "" then state.mode = "legacy_shadow" end
    if trim(state.phase) == "" then state.phase = "shadow" end
    return persist_binding_lowering_state(trim(binding.binding_id), state)
end

M._revalidate_binding_dispatch = function(binding: any, _context: any): (any?, any)
    local b = type(binding) == "table" and (binding :: automation_types.Map) or {}
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component module unavailable for binding dispatch revalidation" end

    local policy = type(b.execution_policy) == "table" and (b.execution_policy :: automation_types.Map) or {}
    local mode = trim(policy.mode)
    if mode == "" then mode = "component_owner" end

    local subject_id = ""
    if mode ~= "component_owner" then
        return nil, "unsupported binding execution_policy.mode: " .. mode
    end
    if type((component :: any).component_owner) ~= "function" then
        return nil, "component owner lookup unavailable for binding dispatch revalidation"
    end
    local owner, owner_err = (component :: any).component_owner(trim(b.binding_id))
    if owner_err then return nil, "binding component owner lookup failed: " .. tostring(owner_err) end
    subject_id = trim(owner)
    if subject_id == "" then return nil, "binding component owner unavailable: " .. trim(b.binding_id) end

    if type((component :: any).get_private_context) ~= "function" then
        return nil, "binding execution identity lookup unavailable"
    end
    local ctx, ctx_err = (component :: any).get_private_context(trim(b.binding_id))
    if ctx_err then return nil, "binding execution identity lookup failed: " .. tostring(ctx_err) end
    if type(ctx) ~= "table" then return nil, "binding execution identity unavailable: " .. trim(b.binding_id) end
    local identity = (ctx :: automation_types.Map)[EXECUTION_IDENTITY_KEY]
    if type(identity) ~= "table" then return nil, "binding execution identity unavailable: " .. trim(b.binding_id) end

    local app_scope: any = nil
    if type(b.authority_scope) == "table" then
        app_scope = (b.authority_scope :: automation_types.Map).app_scope or (b.authority_scope :: automation_types.Map).scope
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
            binding_id = trim(b.binding_id),
            installed_by = trim(b.created_by) ~= "" and trim(b.created_by) or nil,
        },
    }, nil
end

M._launch_flow = function(args: any, authority: any?): (any?, any)
    local a = type(args) == "table" and (args :: automation_types.Map) or {}
    local flow_ref = type(a.flow_ref) == "table" and (a.flow_ref :: automation_types.Map) or {}
    local funcs_mod = mod("funcs")
    if not funcs_mod or type((funcs_mod :: any).new) ~= "function" then return nil, "funcs module unavailable" end
    local executor = (funcs_mod :: any).new()
    if type(authority) == "table" and (authority :: automation_types.Map).actor ~= nil
        and (authority :: automation_types.Map).scope ~= nil then
        executor = executor:with_actor((authority :: automation_types.Map).actor)
            :with_scope((authority :: automation_types.Map).scope)
    end
    local kind = tostring(flow_ref.kind or "")
    if kind == "workflow_definition" then
        return executor:call(WORKFLOW_EXECUTE, {
            workflow_ref = flow_ref,
            input = type(a.input) == "table" and a.input or {},
            opts = {
                mode = a.mode,
                idempotency_key = a.idempotency_key,
                trace_context = a.trace_context,
                authority_identity = a.authority_identity,
                trigger_kind = a.trigger_kind,
                trigger_id = a.trigger_id,
                invocation = a.invocation,
            },
        })
    end
    if kind == "behavior" then
        return executor:call(BEHAVIOR_INVOKE, {
            point_ref = {
                behavior_id = flow_ref.behavior_id,
                component_instance_id = flow_ref.component_instance_id or flow_ref.component_instance,
            },
            input = type(a.input) == "table" and a.input or {},
            opts = { invocation_id = type(a.invocation) == "table" and (a.invocation :: automation_types.Map).invocation_id or nil },
        })
    end
    if kind == "sink" then
        local port_id = tostring(flow_ref.id or "")
        local desc, derr = M.resolve_sink(port_id)
        if derr or type(desc) ~= "table" then return nil, tostring(derr or "sink port not found") end
        local binding = tostring((desc :: automation_types.Map).binding or "")
        if binding == "" then return nil, "sink port has no binding: " .. port_id end
        -- The sink receipt is (origin, sink_op, idempotency_key). idempotency_key alone
        -- names the source item, so the dispatching binding rides along as the canonical
        -- writer origin; without it two bindings writing one source key collapse into
        -- one effect at the sink.
        local output, sink_err = M.dispatch_to_sink(binding, type(a.config) == "table" and a.config or {}, {
            config = type(a.config) == "table" and a.config or {},
            input = type(a.input) == "table" and a.input or {},
            sink_op = tostring(a.sink_op or "upsert"),
            idempotency_key = a.idempotency_key,
            origin = trim(a.trigger_id) ~= "" and trim(a.trigger_id) or nil,
            trace_context = a.trace_context,
        })
        if sink_err then return nil, sink_err end
        -- A destination that answers without acknowledging success refused the
        -- write. Reporting that as a completed launch would advance the cursor
        -- over an effect that never landed, so the refusal is a failed launch and
        -- the destination's own classification rides with it.
        local written = type(output) == "table" and (output :: automation_types.Map) or {}
        if written.success ~= true then
            local structured = failure_lib.from_result(written)
            local raw_error: any = written.error
            local message = (type(raw_error) == "table" and ((raw_error :: automation_types.Map).message or (raw_error :: automation_types.Map).code))
                or raw_error or "sink write did not acknowledge success"
            local failed: automation_types.Map = {
                success = false,
                status = "failed",
                error = tostring(message),
                output = output,
            }
            if structured ~= nil then failed.failure = structured end
            return failed, nil
        end
        return { success = true, status = "completed", output = output }, nil
    end
    return nil, "unsupported automation destination: " .. kind
end

-- A due cron row can already be claimed while DELETE (or pause) synchronously
-- reaps the binding-owned lowering. The binding snapshot used to map the fire is
-- therefore not enough authority to launch a run: re-read the small binding row
-- at the launch boundary for timer fires.
local function periodic_fire_dispatchable(binding_id: string): (boolean?, string?, any)
    local current, read_err = get_binding_row(binding_id)
    if read_err then return nil, nil, read_err end
    if not current then return false, "binding_missing", nil end
    if current.enabled ~= true then return false, "disabled", nil end
    return true, nil, nil
end

local function skip_periodic_fire(binding_id: string, reason: string): automation_types.Map
    local logger_mod = mod("logger")
    if logger_mod and type((logger_mod :: any).named) == "function" then
        (logger_mod :: any):named("automations.binding_dispatch"):debug(
            "periodic binding is no longer dispatchable; dropping claimed fire", {
                binding_id = binding_id,
                reason = reason,
            })
    end
    -- This is a terminal skip for the already-claimed fire. Returning success
    -- prevents the scheduler from retrying a binding its owner has removed (or
    -- paused); DELETE has already removed the recurring cron row itself.
    return { success = true, skipped = true, terminal = true, reason = reason }
end

function M.execute_binding(args: any): (automation_types.Map?, any)
    local a = type(args) == "table" and (args :: automation_types.Map) or {}
    local binding_id = trim(a.binding_id)
    if binding_id == "" then binding_id = trim(a.trigger_id) end
    if binding_id == "" then return nil, "binding_id is required" end
    if type(a.envelopes) == "table" then
        local failed_keys: { string } = {}
        -- Keyed delivery failures carry two things forward: the sentence the
        -- durable last_error shows, and the structure the poison ledger decides
        -- from. Both are kept here, while the envelope still has its key and the
        -- launch still has its own classification.
        local failure_details: { automation_types.Map } = {}
        local delivered = 0
        for _, envelope in ipairs(a.envelopes :: { any }) do
            local env = type(envelope) == "table" and (envelope :: automation_types.Map) or {}
            local res, err = M.execute_binding({
                binding_id = binding_id,
                context = env.item,
                occurred_at = env.occurred_at,
                event_type = env.event_type,
                dedup_key = env.dedup_key,
                trace_context = env.trace_context,
            })
            if err or (type(res) == "table" and (res :: automation_types.Map).success == false) then
                local key = trim(env.dedup_key)
                if key == "" then return nil, err or tostring((res :: automation_types.Map).error or "binding execution failed") end
                failed_keys[#failed_keys + 1] = key
                local result_map = type(res) == "table" and (res :: automation_types.Map) or {}
                local detail: automation_types.Map = {
                    dedup_key = key,
                    error = tostring(err or result_map.error or "binding execution failed"),
                }
                -- The runnable's own structure when it stated one, else the error
                -- object itself when that object IS one. A launch that stated
                -- neither leaves the detail unstructured, which downstream reads
                -- as "no classification", never as a classification of its own.
                local structured = failure_lib.normalize(result_map.failure)
                if structured == nil then structured = failure_lib.normalize(result_map.error) end
                if structured ~= nil then detail.failure = structured end
                failure_details[#failure_details + 1] = detail
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

    local binding, berr = get_binding_row(binding_id)
    if berr then return nil, berr end
    local periodic_fire = a.event_type == "schedule"
    if not binding then
        if periodic_fire then return skip_periodic_fire(binding_id, "binding_missing"), nil end
        return nil, "binding not found: " .. binding_id
    end
    if binding.enabled ~= true then return { success = true, skipped = true, reason = "disabled" }, nil end

    local raw_context = a.context or a.item or {}
    local input = mapping_input_context(binding, raw_context, a)
    local keep, gerr = binding_guard_allows(binding, input)
    if gerr then return nil, gerr end
    if not keep then return { success = true, skipped = true, reason = "guard" }, nil end
    local mapped, merr = apply_binding_mapping(binding.mapping_spec, input)
    if merr then return nil, merr end

    local authority, aerr = M._revalidate_binding_dispatch(binding, input)
    if aerr or not authority then return nil, aerr or "binding dispatch revalidation failed" end
    local auth = type(authority) == "table" and (authority :: automation_types.Map) or {}

    local trace_context = type(a.trace_context) == "table" and a.trace_context
        or (type(input.trace_context) == "table" and input.trace_context or nil)
    local trigger_id = trim(binding.legacy_trigger_id)
    if trigger_id == "" then trigger_id = binding.binding_id end
    local policy = type(binding.execution_policy) == "table" and (binding.execution_policy :: automation_types.Map) or {}
    local launch_args: automation_types.Map = {
        flow_ref = binding.flow_ref,
        input = type(mapped) == "table" and mapped or { value = mapped },
        mode = trim(policy.launch_mode) ~= "" and policy.launch_mode or "spawned_child_run",
        idempotency_key = trim(a.dedup_key) ~= "" and trim(a.dedup_key) or nil,
        trace_context = trace_context,
        authority_identity = auth.authority_identity,
        trigger_kind = "binding",
        trigger_id = trigger_id,
        invocation = {
            source = "binding",
            binding_id = binding.binding_id,
            trigger_id = binding.trigger_id,
            invocation_id = trim(a.invocation_id) ~= "" and trim(a.invocation_id) or nil,
            authority_identity = auth.authority_identity,
            audit = {
                installed_by = trim(binding.created_by) ~= "" and trim(binding.created_by) or nil,
            },
        },
    }
    if shadow_compare_enabled(binding) then
        local recorded, rerr = record_shadow_compare(binding, launch_args, input, mapped, a)
        if rerr then return nil, "binding shadow compare record failed: " .. tostring(rerr) end
        return {
            success = true,
            shadow_compare = true,
            binding_id = binding.binding_id,
            would_launch = launch_args,
            lowering_state = recorded and (recorded :: automation_types.Map).lowering_state or nil,
        }, nil
    end
    if periodic_fire then
        local dispatchable, reason, fire_err = periodic_fire_dispatchable(binding.binding_id)
        if fire_err then return nil, fire_err end
        if dispatchable ~= true then
            return skip_periodic_fire(binding.binding_id, reason or "binding_missing"), nil
        end
    end
    local out, lerr = M._launch_flow(launch_args, auth)
    if lerr then return nil, lerr end
    local result = type(out) == "table" and (out :: automation_types.Map) or {}
    result.success = result.success ~= false
    return result, nil
end

M._read_legacy_trigger_context = function(workflow_id: string): (automation_types.Map?, any)
    local state, err = M.read_state(workflow_id)
    if err then return nil, err end
    local s = type(state) == "table" and (state :: automation_types.Map) or {}
    local trigger = type(s[TRIGGER_STATE_KEY]) == "table" and (s[TRIGGER_STATE_KEY] :: automation_types.Map) or nil
    if not trigger then return nil, "legacy workflow trigger not found: " .. workflow_id end
    return {
        title = trim(s.title) ~= "" and s.title or workflow_id,
        trigger = trigger,
        enabled = s.enabled ~= false,
        authority_scope = type(s.authority_scope) == "table" and s.authority_scope or {},
    }, nil
end

local function normalize_legacy_filter_expr(expr: any): string?
    local source = trim(expr)
    if source == "" then return nil end
    source = source:gsub("envelope%.item", "input")
    source = source:gsub("^item%.", "input.")
    source = source:gsub("([^%w_])item%.", "%1input.")
    if source == "item" then return "input" end
    source = source:gsub("^item([^%w_])", "input%1")
    source = source:gsub("([^%w_])item([^%w_])", "%1input%2")
    source = source:gsub("([^%w_])item$", "%1input")
    return source
end

local function trigger_config_from_legacy_spec(spec: automation_types.Map): automation_types.Map
    local config = type(spec.config) == "table" and copy_map(spec.config) or {}
    if spec.schedule ~= nil then config.schedule = spec.schedule end
    if spec.backfill ~= nil then config.backfill = spec.backfill end
    return config
end

function M.create_workflow_trigger_shadow_binding(input: any): (automation_types.Map?, any)
    local args = type(input) == "table" and (input :: automation_types.Map) or {}
    local workflow_id = trim(args.workflow_id)
    if workflow_id == "" then return nil, "workflow_id is required" end
    local legacy, lerr = M._read_legacy_trigger_context(workflow_id)
    if lerr or not legacy then return nil, lerr or "legacy workflow trigger unavailable" end
    local l = legacy :: automation_types.Map
    local trigger = type(l.trigger) == "table" and (l.trigger :: automation_types.Map) or {}
    local spec = type(trigger.spec) == "table" and (trigger.spec :: automation_types.Map) or trigger
    local source = trim(spec.source)
    if source == "" then return nil, "legacy workflow trigger source is required" end

    local filter = type(spec.filter) == "table" and (spec.filter :: automation_types.Map) or {}
    local version = tonumber(args.workflow_version)
    local legacy_enabled = l.enabled ~= false
    local spec_input: automation_types.Map = {
        binding_id = trim(args.binding_id) ~= "" and trim(args.binding_id) or nil,
        portable_key = trim(args.portable_key) ~= "" and trim(args.portable_key) or ("legacy.workflow_trigger/" .. workflow_id),
        title = trim(args.title) ~= "" and trim(args.title) or ("Trigger binding: " .. tostring(l.title or workflow_id)),
        enabled = false,
        trigger_id = source,
        trigger_config = trigger_config_from_legacy_spec(spec),
        flow_ref = {
            kind = "workflow_definition",
            id = workflow_id,
            version = version,
        },
        mapping_spec = { v = 1, mode = "expr", expr = "input" },
        guard_expr = normalize_legacy_filter_expr(filter.expr),
        execution_policy = {
            mode = "component_owner",
            launch_mode = "spawned_child_run",
            shadow_compare = true,
        },
        authority_scope = type(args.authority_scope) == "table" and args.authority_scope
            or (type(l.authority_scope) == "table" and l.authority_scope or {}),
        legacy_trigger_id = workflow_id,
    }

    local created, cerr = M.create_binding(spec_input, trim(args.parent_id) ~= "" and trim(args.parent_id) or nil)
    if cerr or not created then return nil, cerr or "create shadow binding failed" end
    local lowering_state = {
        v = 1,
        mode = "legacy_shadow",
        phase = "shadow",
        component_id = (created :: automation_types.Map).binding_id,
        trigger_id = source,
        migration = {
            kind = "workflow_trigger",
            phase = "shadow",
            workflow_id = workflow_id,
            legacy_trigger_id = workflow_id,
            legacy_enabled = legacy_enabled,
            legacy_mode = trigger.mode,
            legacy_spec = spec,
            legacy_registration = type(trigger.registration) == "table" and trigger.registration or {},
        },
    }
    return persist_binding_enabled((created :: automation_types.Map).binding_id, legacy_enabled, lowering_state)
end

function M.execute_legacy_trigger_shadow_bindings(legacy_trigger_id: string, envelopes: any): (automation_types.Map?, any)
    local id = trim(legacy_trigger_id)
    if id == "" then return nil, "legacy_trigger_id is required" end
    local bindings, berr = M.list_bindings({ legacy_trigger_id = id, enabled = true })
    if berr then return nil, berr end
    local compared = 0
    local failures: { automation_types.Map } = {}
    for _, binding in ipairs(bindings or {}) do
        local b = binding :: automation_types.Map
        if shadow_compare_enabled(b) then
            local out, err = M.execute_binding({ binding_id = b.binding_id, envelopes = type(envelopes) == "table" and envelopes or {} })
            if err or (type(out) == "table" and (out :: automation_types.Map).success == false) then
                failures[#failures + 1] = { binding_id = b.binding_id, error = tostring(err or (out :: automation_types.Map).error or "shadow compare failed") }
            else
                compared = compared + (tonumber((out :: automation_types.Map).delivered) or 0)
            end
        end
    end
    return { success = true, compared = compared, failures = failures }, nil
end

function M.execute_binding_action(args: any): (automation_types.Map?, any)
    local a = type(args) == "table" and (args :: automation_types.Map) or {}
    local id = trim(a.binding_id)
    if id == "" then
        local ctx_mod = mod("ctx")
        if ctx_mod then id = trim((ctx_mod :: any).get("component_id")) end
    end
    local context = a.context
    local fire_timestamp: string? = nil
    if context == nil and type(a._schedule) == "table" then
        local sched = a._schedule :: automation_types.Map
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
        local sched = a._schedule :: automation_types.Map
        local schedule_id = trim(sched.schedule_id)
        local fired_at = fire_timestamp or trim(sched.fired_at)
        if schedule_id ~= "" and fired_at ~= "" then schedule_dedup = schedule_id .. ":" .. fired_at end
    end
    return M.execute_binding({
        binding_id = id,
        context = context or {},
        occurred_at = type(context) == "table" and (context :: automation_types.Map).occurred_at or nil,
        event_type = type(a._schedule) == "table" and "schedule" or nil,
        dedup_key = a.dedup_key or schedule_dedup,
        trace_context = a.trace_context,
    })
end

function M.poll_binding_action(args: any): (automation_types.Map?, any)
    -- Poll cursor parity is M5's reconcile gate. M3 stores the poll lowering and
    -- cron route, but real side-effect dispatch is blocked by the same authority
    -- revalidation gap as execute_binding.
    local _ = args
    return { success = false, error = "poll binding execution is blocked until dispatch-time authority revalidation exists", retriable = false }, nil
end

-- resolve_io_port recovers a port descriptor by its PORT ENTRY id — the
-- descriptor the run path (write_to_sink / read_from_source) needs to act through
-- the port's backing. Stored specs carry port entry ids (v3 cutover), so the
-- match is by `id` only. Returns nil with an error when no port declares the id.
local function resolve_io_port(key: string, dir: string, label: string): (automation_types.Map?, string?)
    if type(key) ~= "string" or key == "" then return nil, label .. " port is required" end
    local list, err = list_io(dir)
    if err then return nil, tostring(err) end
    for _, raw in ipairs(list or {}) do
        local desc = raw :: automation_types.Map
        if tostring(desc.id or "") == key then
            return (desc :: automation_types.Map?), nil
        end
    end
    return nil, label .. " port not found: " .. key
end

-- resolve_sink / resolve_source: the port descriptor an automation stored
-- under its sink / source port id, recovered at run time.
function M.resolve_sink(port_id: string): (automation_types.Map?, string?)
    local desc, err = resolve_io_port(port_id, "in", "sink")
    return (desc :: automation_types.Map?), err
end

function M.resolve_source(port_id: string): (automation_types.Map?, string?)
    local desc, err = resolve_io_port(port_id, "out", "source")
    return (desc :: automation_types.Map?), err
end

-- The deterministic per-actor component id for a class. A per-user singleton
-- source component (uploads, inbox) has exactly one instance, so a trigger on it
-- resolves the instance from the installing actor + class instead of prompting a
-- picker that would only ever offer one choice.
function M.component_id_for_actor(class: string): (string?, string?)
    if type(class) ~= "string" or class == "" then return nil, "source declares no class" end
    local sec = mod("security")
    local actor = sec and (sec :: any).actor()
    if not actor then return nil, "authentication required" end
    local actor_id = (actor :: any):id()
    if type(actor_id) ~= "string" or actor_id == "" then return nil, "actor id unavailable" end
    local id = (mod("autoinit") :: any).component_id(actor_id, class)
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
local function sink_open_context(config: automation_types.Map): automation_types.Map
    local context = copy_map(config) :: any
    local component_id = trim((context :: any).component_id)
    if component_id == "" then component_id = trim((context :: any).thread_id) end
    if component_id ~= "" then (context :: any).component_id = component_id end
    return context :: automation_types.Map
end

function M.open_sink_writer(binding: string, config: automation_types.Map): (any?, string?)
    if type(binding) ~= "string" or binding == "" then return nil, "sink binding is required" end
    local contract_mod = mod("contract")
    if not contract_mod then return nil, "contract module unavailable" end
    local def, derr = contract_mod.get(WRITABLE_CONTRACT)
    if derr or not def then return nil, "writable unavailable: " .. tostring(derr) end
    local context = sink_open_context(config or {})
    local inst, oerr = (def :: any):with_context(context):open(binding)
    if oerr or not inst then return nil, "writable open: " .. tostring(oerr) end
    return {
        write = function(_self: any, body: automation_types.Map): (any?, string?)
            local res, werr = (inst :: any):write(body)
            if werr then return nil, tostring(werr) end
            if type(res) ~= "table" then
                return nil, "sink write returned a non-object result"
            end
            local result = res :: automation_types.Map
            if result.success ~= true then
                local rerr: any = result.error
                local emsg = (type(rerr) == "table" and (rerr.message or rerr.code)) or rerr
                    or "sink write did not acknowledge success=true"
                return nil, tostring(emsg)
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
function M.dispatch_to_sink(binding: string, config: automation_types.Map, body: automation_types.Map): (any?, string?)
    local writer, err = M.open_sink_writer(binding, config)
    if err or not writer then return nil, err end
    return (writer :: any):write(body)
end


-- list_installed: read umbrella components from userspace, project state,
-- merge reportable.status. opts: { actor_id? = string, class? = string,
-- limit? = number, offset? = number }.
local function flow_ref_for_list_row(component_id: any, impl_id: any, meta: automation_types.Map): automation_types.Map?
    if type(meta.flow_ref) == "table" then return stored_flow_ref(meta.flow_ref) end
    if impl_id ~= AUTOMATION_BINDING_KIND then return nil end
    -- Binding shell metadata gained flow_ref after bindings already existed.
    -- Read the durable spec on demand for those old shells; no request-time write
    -- or migration is needed to give callers the canonical projection.
    local binding = select(1, get_binding_row(tostring(component_id or "")))
    return binding and binding.flow_ref or nil
end

-- Older binding shells predate materialized public state. Read their durable
-- rows once per list request so upgraded installations immediately expose the
-- same canonical state as newly created bindings. New shells stay query-free.
local function binding_public_state_backfill(components: { automation_types.ComponentRow }): ({ [string]: automation_types.Map }?, any)
    local needed = false
    for _, c in ipairs(components) do
        local meta: automation_types.Map = c.meta or {}
        if c.impl_id == AUTOMATION_BINDING_KIND and (meta.enabled == nil or meta.status == nil) then
            needed = true
            break
        end
    end
    if not needed then return {}, nil end
    local db, db_err = binding_db()
    if not db then return nil, db_err end
    local rows, query_err = (db :: any):query(
        "SELECT binding_id, enabled FROM " .. AUTOMATION_BINDING_TABLE,
        {})
    if type((db :: any).release) == "function" then (db :: any):release() end
    if query_err then return nil, query_err end
    local out: { [string]: automation_types.Map } = {}
    for _, row in ipairs(rows or {}) do
        local enabled = is_enabled_value((row :: automation_types.Map).enabled)
        out[tostring((row :: automation_types.Map).binding_id)] = {
            enabled = enabled,
            status = enabled and "idle" or "paused",
        }
    end
    return out, nil
end

function M.list_installed(opts: automation_types.ListInstalledOptions?): ({automation_types.InstalledAutomation}?, any)
    opts = opts or {}
    local limit, limit_err = pagination_int(opts.limit, 200, 1, 500, "limit")
    if limit_err then return nil, limit_err end
    local offset, offset_err = pagination_int(opts.offset, 0, 0, nil, "offset")
    if offset_err then return nil, offset_err end

    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return nil, "registry unavailable" end

    -- Read every umbrella by binding impl id (any class), then narrow with
    -- class_matches below. Filtering the query by meta.class would drop
    -- umbrellas whose class isn't the literal "automation" (e.g. knowledge).
    local impl_ids = automation_binding_ids()
    if #impl_ids == 0 then return {}, nil end

    -- Actor-scoped read goes through the access-filtered read-port; the no-actor
    -- path is a trusted system listing (e.g. background reconciliation).
    local components: { automation_types.ComponentRow }
    if type(opts.actor_id) == "string" and opts.actor_id ~= "" then
        components = (component.query({
            actor_id = opts.actor_id,
            impl_ids = impl_ids,
            parent_id = type(opts.parent_id) == "string" and opts.parent_id ~= "" and opts.parent_id or nil,
            include = { meta = true, access = true, placement = true },
            order_by = { field = "created_at", direction = "DESC" },
            limit = limit,
            offset = offset,
        }) or {}) :: { automation_types.ComponentRow }
    else
        components = (component.list_system({
            impl_ids = impl_ids,
            parent_id = type(opts.parent_id) == "string" and opts.parent_id ~= "" and opts.parent_id or nil,
            include = { meta = true, placement = true },
            order_by = { field = "created_at", direction = "DESC" },
            limit = limit,
            offset = offset,
        }) or {}) :: { automation_types.ComponentRow }
    end
    local binding_state, binding_state_err = binding_public_state_backfill(components)
    if binding_state_err then return nil, binding_state_err end
    -- Per-impl_id public_state_schema cache: every component of one type shares
    -- the same schema, so it is resolved once, not per row. A cache value of
    -- false marks a type that declares no schema (distinct from "not yet looked
    -- up") so that too is memoized. The materialized public_state already lives
    -- in c.meta (include = meta above) -- projecting it is pure, no extra query.
    local schema_cache: { [string]: any } = {}
    local out: {automation_types.InstalledAutomation} = {}
    for _, c in ipairs(components) do
        local meta_t: automation_types.Map = c.meta or {}
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
            local flow_ref = flow_ref_for_list_row(c.component_id, c.impl_id, meta_t)
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
                    local projected = project_public_state(cached, state_meta)
                    if projected then entry.public_state = projected :: automation_types.Map end
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
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end

    local impl_ids: {string}
    if type(opts.type_ids) == "table" and #(opts.type_ids :: {string}) > 0 then
        impl_ids = opts.type_ids :: {string}
    elseif type(opts.type_meta) == "table" then
        impl_ids = binding_ids_by_meta(opts.type_meta)
    else
        impl_ids = automation_binding_ids()
    end
    if #impl_ids == 0 then return {}, nil end

    local limit, limit_err = pagination_int(opts.limit, 200, 1, 500, "limit")
    if limit_err then return nil, limit_err end
    local offset, offset_err = pagination_int(opts.offset, 0, 0, nil, "offset")
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
        local meta_t: automation_types.Map = c.meta or {}
        if M.class_matches(meta_t.class, opts.class) then
            local ctx = component.get_private_context(c.component_id)
            local identity: any = nil
            local state: any = nil
            if type(ctx) == "table" then
                identity = (ctx :: automation_types.Map)[EXECUTION_IDENTITY_KEY]
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
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end
    local rows = (component.list_system({
        component_ids = { id :: string },
        include = { meta = true },
        limit = 1,
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
        execution_identity = (ctx :: automation_types.Map)[EXECUTION_IDENTITY_KEY],
        state = state,
    } :: automation_types.RuntimeAutomation), nil
end

-- read_public_state is the frontend-safe public state helper.
-- The storage plane is component public meta: schema fields are normal meta keys
-- so list/render/realtime all read one public component surface. Scheduler state
-- stays behind cron; private runtime state stays in private_context.
function M.read_public_state(component_id: string): (automation_types.Map?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local component = mod("component") :: automation_types.ComponentModule?
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not component or not registry then return nil, "platform modules unavailable" end
    local binding_rows, binding_query_err = component.query({
        component_ids = { component_id },
        include = { meta = true },
        access_mask = component.ACCESS.READ,
        limit = 1,
    })
    if binding_query_err then return nil, binding_query_err end
    local binding_row = binding_rows and binding_rows[1]
    if binding_row and binding_row.impl_id == AUTOMATION_BINDING_KIND then
        local binding, binding_err = M.get_binding(component_id)
        if binding_err or not binding then return nil, binding_err or "automation not found" end
        return binding_public_state(binding), nil
    end
    local schema, row, schema_err = public_state_component(component, registry, component_id, component.ACCESS.READ)
    if schema_err then return nil, schema_err end
    local projected, project_err = project_public_state(schema, row and row.meta or {})
    if project_err then return nil, project_err end
    local out: automation_types.Map
    if projected then
        out = (projected :: automation_types.Map)
    else
        out = {}
    end
    return (out :: automation_types.Map), nil
end

-- write_public_state partially updates a component's public state (its render/read
-- model) through component public meta, validating each provided field against the
-- type's public_state_schema. WRITE-gated. Automation control methods (pause/resume,
-- status transitions) use it so the card and list reflect a state change live via the
-- component.meta.changed relay, without owning a projection.
function M.write_public_state(component_id: string, patch: any): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(patch) ~= "table" then return nil, "patch must be a table" end
    local component = mod("component") :: automation_types.ComponentModule?
    local registry = mod("registry") :: automation_types.RegistryModule?
    if not component or not registry then return nil, "platform modules unavailable" end
    local schema, _, schema_err = public_state_component(component, registry, component_id, component.ACCESS.WRITE)
    if schema_err then return nil, schema_err end
    local allowed_map, fields_err = public_state_field_map(schema)
    if fields_err then return nil, fields_err end
    local allowed: { [string]: any } = allowed_map or {}

    -- title/comment are core component-meta fields (not schema-declared read-model
    -- fields); a reconfigure that renames writes them alongside its public_state.
    local CORE_META: { [string]: boolean } = { title = true, comment = true }
    local fields: automation_types.Map = {}
    for k, v in pairs(patch :: automation_types.Map) do
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
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end
    local access = required_access or component.ACCESS.READ
    local ctx, err = component.get_context(component_id, access)
    if err then return nil, err end
    return copy_private_state(ctx, true), nil
end

-- ── the stuck-item operator surface ──────────────────────────────────────
-- An item a trigger cannot get past is shown on the automation it belongs to and
-- answered there. Reading the alerts needs read access to that automation and
-- commanding one needs write access -- the same authority that installed it --
-- while the row itself stays owned by the automation's frozen owner identity,
-- which is who the alert is for.
M._stuck_item = stuck_item

function M.list_stuck_items(component_id: string): ({ automation_types.Map }?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local _, access_err = M.read_state(component_id)
    if access_err then return nil, access_err end
    local rows, err = M._stuck_item.list(component_id)
    if err then return nil, tostring(err) end
    return rows, nil
end

function M.request_stuck_item_command(args: any): (automation_types.Map?, any)
    local a = type(args) == "table" and (args :: automation_types.Map) or {}
    local component_id = trim(a.component_id)
    if component_id == "" then return nil, "component_id is required" end
    local item_key = trim(a.item_key)
    if item_key == "" then return nil, "item_key is required" end
    local action = trim(a.action)
    if M._stuck_item.REQUESTED_STATE_FOR[action] == nil then
        return nil, "action must be one of retry, skip"
    end

    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end
    local _, access_err = M.read_state(component_id, component.ACCESS.WRITE)
    if access_err then return nil, access_err end

    local sec = mod("security")
    local actor = sec and (sec :: any).actor()
    local requested_by = actor and tostring((actor :: any):id() or "") or ""
    if requested_by == "" then return nil, "authentication required" end

    local row, err = M._stuck_item.request({
        binding_id = component_id,
        item_key = item_key,
        action = action,
        requested_by = requested_by,
    })
    if err then return nil, tostring(err) end
    return row, nil
end

-- read_config returns the FULL editable install config a reconfigurable type
-- stores as its own private state (code/tool_ids/trigger/…) so an editor can
-- repopulate the create view on re-open. WRITE-gated and limited to types that
-- declare meta.reconfigurable, so only opt-in types expose their private config;
-- the live component title overlays state.title so a prior rename survives edit.
function M.read_config(component_id: string): (automation_types.Map?, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    local registry = mod("registry") :: automation_types.RegistryModule?
    local component = mod("component") :: automation_types.ComponentModule?
    if not registry or not component then return nil, "platform modules unavailable" end

    local rows, query_err = component.query({
        component_ids = { component_id },
        include = { meta = true },
        access_mask = component.ACCESS.WRITE,
        limit = 1,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then return nil, "automation not found" end

    if row.impl_id == AUTOMATION_BINDING_KIND then
        local binding, binding_err = M.get_binding(component_id)
        if binding_err or not binding then return nil, binding_err or "automation not found" end
        return binding_config(binding), nil
    end

    local raw_entry, get_err = registry.get(row.impl_id :: string)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err or not entry or entry.kind ~= "contract.binding" then return nil, "automation type not found" end
    local meta: automation_types.Map = entry.meta or {}
    if meta.type ~= AUTOMATION_TYPE then return nil, "binding is not an automation type" end
    if meta.reconfigurable ~= true then return nil, "automation type does not support reconfigure" end

    local state, state_err = M.read_state(component_id, component.ACCESS.WRITE)
    if state_err or type(state) ~= "table" then return nil, state_err or "automation not found" end
    local out: automation_types.Map = state :: automation_types.Map
    local live_title = row.meta and (row.meta :: automation_types.Map).title
    if type(live_title) == "string" and live_title ~= "" then out.title = live_title end
    return out, nil
end

local TRIGGER_GENERATION_PATH = TRIGGER_STATE_KEY .. ".state_generation"
local STATE_WRITE_ATTEMPTS = 4

-- The compare-and-set a writer states about the trigger block it just read. A
-- block that carries a generation is compared on it; a block that does not
-- (installed before the block became a document) is compared on the ABSENCE of
-- the generation, so the first write under the new discipline is still a race
-- exactly one writer can win; no block at all is compared on absence of the key.
-- The second value is the generation the write lands with.
function M.trigger_precondition(context: any): automation_types.Map
    local block = type(context) == "table" and (context :: automation_types.Map)[TRIGGER_STATE_KEY] or nil
    if type(block) ~= "table" then return { expect_absent = true } end
    local generation = tonumber((block :: automation_types.Map).state_generation)
    if generation == nil then return { expect_generation_absent = true } end
    return { expect_generation = math.floor(generation) }
end

-- The precondition object the component service evaluates, and the generation the
-- block being written takes.
local function trigger_precondition_sql(opts: any): (automation_types.Map?, number)
    local options = type(opts) == "table" and (opts :: automation_types.Map) or {}
    if options.expect_absent == true then
        return { expect_absent = { TRIGGER_STATE_KEY } }, 1
    end
    if options.expect_generation_absent == true then
        return { expect_absent = { TRIGGER_GENERATION_PATH } }, 1
    end
    if options.expect_generation ~= nil then
        local seen = math.floor(tonumber(options.expect_generation) or 0)
        return { expect_match = { [TRIGGER_GENERATION_PATH] = seen } }, seen + 1
    end
    return nil, 0
end

function M.patch_state(component_id: string, patch: any, opts: automation_types.PatchStateOptions?): (any, any)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if type(patch) ~= "table" then return nil, "patch must be a table" end
    if (patch :: automation_types.Map)._rollback ~= nil then
        return nil, "invalid patch: _rollback is managed by the automation runtime"
    end
    if (patch :: automation_types.Map).component_id ~= nil then
        return nil, "invalid patch: component_id is managed by the automation runtime"
    end
    if (patch :: automation_types.Map).public_state ~= nil then
        return nil, "invalid patch: public_state is managed by component public meta"
    end
    if (patch :: automation_types.Map)[EXECUTION_IDENTITY_KEY] ~= nil then
        return nil, "invalid patch: " .. EXECUTION_IDENTITY_KEY .. " is managed by the automation runtime"
    end
    if (patch :: automation_types.Map)[TRIGGER_STATE_KEY] ~= nil then
        return nil, "invalid patch: " .. TRIGGER_STATE_KEY .. " is managed by the trigger service"
    end

    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end

    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end

    -- A patch replaces the WHOLE context, including the trigger block it merely
    -- read. It therefore states the block generation it saw: a tick that
    -- committed a cursor between this read and this write wins, and the patch
    -- re-reads and reapplies itself onto what the tick left.
    for _ = 1, STATE_WRITE_ATTEMPTS do
        local current, read_err = component.get_context(component_id, component.ACCESS.WRITE)
        if read_err then return nil, read_err end
        local rollback: any = nil
        local frozen_identity: any = nil
        if type(current) == "table" then
            rollback = (current :: automation_types.Map)._rollback
            frozen_identity = (current :: automation_types.Map)[EXECUTION_IDENTITY_KEY]
        end
        local next_state = copy_private_state(current, true)

        for k, v in pairs(patch :: automation_types.Map) do
            next_state[k] = v
        end
        if opts and type(opts.delete_keys) == "table" then
            for _, key in ipairs(opts.delete_keys) do
                if type(key) == "string" and key ~= "_rollback" and key ~= "component_id"
                    and key ~= EXECUTION_IDENTITY_KEY and key ~= TRIGGER_STATE_KEY then
                    next_state[key] = nil
                end
            end
        end
        if rollback ~= nil then next_state._rollback = rollback end
        if frozen_identity ~= nil then next_state[EXECUTION_IDENTITY_KEY] = frozen_identity end

        local precondition = trigger_precondition_sql(M.trigger_precondition(current))
        local payload: automation_types.Map = { private_context = next_state }
        if precondition ~= nil then payload.precondition = precondition end
        local result, update_err = svc:update({
            component_id = component_id,
            commands = {
                { type = "SET_CONTEXT", payload = payload },
            },
        })
        if update_err then return nil, update_err end
        if result and result.success then
            return { success = true, id = component_id, state = copy_private_state(next_state, true) }, nil
        end
        local conflicted = type(result) == "table" and (result :: automation_types.Map).error_kind == errors.CONFLICT
        if not conflicted then
            return nil, tostring(result and result.error or "state update failed")
        end
    end
    return nil, "state update lost the trigger block race repeatedly for component " .. component_id
end

-- write_trigger_state is the single writer for the engine-reserved _trigger key:
-- the trigger service (kickside.automation.trigger) stores its bookkeeping block
-- ({ v, spec, mode, registration, phase, state_generation, install_id }) here, and
-- nothing else touches the key (patch_state refuses it in both patch and
-- delete_keys). block = nil clears the key (trigger uninstall). Every other
-- private-context field, the rollback chain, and the frozen identity pass through
-- verbatim.
--
-- The block is a compare-and-set document. `opts.expect_generation` states the
-- generation the caller read and the write lands only while that still stands,
-- moving it one forward; `opts.expect_absent` installs a first block only while
-- the key does not exist. The comparison happens inside the update's own
-- statement, so no writer can overwrite a commit it never saw. The third return
-- value tells a caller that the refusal WAS such a race, which is the difference
-- between re-reading and reapplying its intent and giving up.
function M.write_trigger_state(component_id: string, block: any, opts: any?): (any, any, boolean?)
    if type(component_id) ~= "string" or component_id == "" then return nil, "id is required" end
    if block ~= nil and type(block) ~= "table" then return nil, "trigger block must be a table or nil" end

    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end

    local precondition, next_generation = trigger_precondition_sql(opts)
    if precondition ~= nil and block ~= nil then
        (block :: automation_types.Map).state_generation = next_generation
    end

    local current, read_err = component.get_context(component_id, component.ACCESS.WRITE)
    if read_err then return nil, read_err end
    local next_state = copy_private_state(current, false)
    next_state[TRIGGER_STATE_KEY] = block

    local svc, svc_err = component.get_service()
    if not svc then return nil, "component service: " .. tostring(svc_err) end
    local payload: automation_types.Map = { private_context = next_state }
    if precondition ~= nil then payload.precondition = precondition end
    local result, update_err = svc:update({
        component_id = component_id,
        commands = {
            { type = "SET_CONTEXT", payload = payload },
        },
    })
    if update_err then return nil, update_err end
    if not result or not result.success then
        local conflicted = precondition ~= nil
            and type(result) == "table"
            and (result :: automation_types.Map).error_kind == errors.CONFLICT
        return nil, tostring(result and result.error or "trigger state update failed"), conflicted
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
    local registry = mod("registry") :: automation_types.RegistryModule?
    local funcs = mod("funcs") :: automation_types.FuncsModule?
    local component = mod("component") :: automation_types.ComponentModule?
    if not registry or not funcs or not component then
        return nil, "platform modules unavailable"
    end

    local raw_entry, get_err = registry.get(type_id)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err or not entry or entry.kind ~= "contract.binding" then
        return nil, "automation type not found"
    end
    local entry_meta: automation_types.Map = entry.meta or {}
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

    local result, call_err = (executor :: any)
        :with_context({ component_id = component_id })
        :call(install_target :: string, (type(input) == "table" and input or {}) :: automation_types.Map)
    if call_err ~= nil then return nil, call_err end
    local shape_err = validate_result(result)
    if shape_err ~= nil then return nil, shape_err end

    local result_t: automation_types.InstallResult = result :: automation_types.InstallResult
    local state: automation_types.Map = result_t.state
    local metadata: automation_types.Map = type(result_t.metadata) == "table" and result_t.metadata or {}
    local public_state: automation_types.Map? = type(result_t.public_state) == "table" and result_t.public_state or nil
    local public_state_err = validate_public_state(entry_meta.public_state_schema, public_state)
    if public_state_err then return nil, public_state_err end
    local rollback, rollback_err = normalize_rollback_chain(result_t.rollback)
    if rollback_err or not rollback then return nil, rollback_err end
    local rollback_chain = rollback :: { automation_types.RollbackStep }

    local kickside_component_meta: automation_types.Map = (safe_component_metadata(metadata, "Untitled") :: automation_types.Map)
    -- class stored as TEXT; flatten an array class to its first (canonical) value.
    local classes = class_array_from_meta(entry_meta)
    if #classes > 0 then kickside_component_meta.class = classes[1] end
    if public_state ~= nil then
        for k, v in pairs(public_state :: automation_types.Map) do
            kickside_component_meta[k] = v
        end
    end

    local private_context: automation_types.AutomationPrivateContext = {}
    for k, v in pairs(state) do private_context[k] = v end
    private_context.component_id = component_id
    private_context._rollback = rollback_chain
    private_context[EXECUTION_IDENTITY_KEY] = execution_identity

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
        rollback = function(target_id: string, args: automation_types.Map?)
            if type(target_id) == "string" and target_id ~= "" then
                rollback_chain[#rollback_chain + 1] = {
                    target = target_id,
                    args = type(args) == "table" and (args :: automation_types.Map) or {},
                }
            end
        end,
    }
    local schedule_ids: { string } = {}
    local created_schedules: { automation_types.Map } = {}
    local declared_schedules: { automation_types.ScheduleSpec } =
        type(result_t.schedules) == "table" and result_t.schedules or {}
    for _, spec in ipairs(declared_schedules) do
        local opts: automation_types.Map = {}
        for k, v in pairs(spec :: automation_types.Map) do opts[k] = v end
        opts.component_id = final_id
        opts.state_key = nil
        local schedule_module = mod("automation_schedule") :: automation_types.Map?
        if not schedule_module then
            M.replay(rollback_chain)
            return nil, "automation schedule helper unavailable"
        end
        local created, sched_err = (schedule_module.create_action_schedule :: any)(opts, schedule_recorder)
        if sched_err or not created or not created.schedule_id then
            M.replay(rollback_chain)
            return nil, "schedule setup failed: " .. tostring(sched_err)
        end
        local created_map = created :: automation_types.Map
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
    local registry = mod("registry") :: automation_types.RegistryModule?
    local component = mod("component") :: automation_types.ComponentModule?
    if not registry or not component then return nil, "platform modules unavailable" end
    local rows, query_err = component.query({
        component_ids = { component_id },
        include = { meta = true },
        access_mask = component.ACCESS.WRITE,
        limit = 1,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row or type(row.impl_id) ~= "string" or row.impl_id == "" then return nil, "automation not found" end
    local raw_entry, get_err = registry.get(row.impl_id :: string)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err or not entry or entry.kind ~= "contract.binding" then return nil, "automation type not found" end
    local entry_meta: automation_types.Map = entry.meta or {}
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
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then
        return nil, "platform modules unavailable"
    end

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
        replay_opts = { options = options :: automation_types.Map }
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

-- delete_automation is the shared HTTP/MCP dispatch point. Binding artifacts do
-- not carry the legacy umbrella rollback ledger, so they enter their own
-- component-kind teardown; legacy automation umbrellas retain uninstall's
-- rollback-before-unregister behavior unchanged.
function M.delete_automation(component_id: string, options: any): (automation_types.Map?, any)
    local id = trim(component_id)
    if id == "" then return nil, "id is required" end
    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end

    local rows, query_err = component.query({
        component_ids = { id },
        include = { meta = true },
        access_mask = component.ACCESS.DELETE,
        limit = 1,
    })
    if query_err then return nil, query_err end
    local row = rows and rows[1]
    if not row then return nil, "automation not found: " .. id end

    if (row :: automation_types.ComponentRow).impl_id == AUTOMATION_BINDING_KIND then
        return M.delete_binding(id)
    end
    return M.uninstall(id, options)
end

local function owned_resource_state_keys(meta: automation_types.Map, resource_kind: string): { string }
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

    local registry = mod("registry") :: automation_types.RegistryModule?
    if not registry then return nil, "registry module unavailable" end
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

    local component = mod("component") :: automation_types.ComponentModule?
    if not component then return nil, "component unavailable" end

    -- System-wide reverse lookup across every owner's umbrellas: a trusted
    -- (unscoped) read, since teardown must find the owner regardless of caller.
    local components = (component.list_system({
        impl_ids = impl_ids,
        include = { private_context = true },
    }) or {}) :: { automation_types.ComponentRow }
    for _, c in ipairs(components or {}) do
        local pc: automation_types.Map = c.private_context or {}
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

-- call_action: dispatch a binding method declared in meta.actions.
-- component.open auto-loads private_context as ctx for the dispatched method.
-- Every refusal this seam makes states its KIND. A caller above it -- the cron
-- adapter most of all -- has to decide whether waiting could change the answer,
-- and a rendered sentence cannot say so: classifying by searching the message for
-- phrases is what let an infrastructure error containing "not found" read as a
-- permanently missing automation and kill a recurring schedule.
local function action_error(message: string, kind: string): error
    return (errors.new({ message = message, kind = kind }) :: error)
end

-- The component gate's own refusal already carries its kind (absence, denial);
-- it travels as it is, and only an untyped value is given one.
local function preserve_kind(err: any, fallback_message: string): error
    if type(err) == "table" or type(err) == "userdata" then
        local e: any = err
        if type(e.kind) == "function" and type(e.message) == "function" then
            return err :: error
        end
    end
    return action_error(fallback_message, errors.INTERNAL)
end

function M.call_action(component_id: string, method_name: string, args: any): (automation_types.CallActionResult?, any)
    if type(component_id) ~= "string" or component_id == "" then
        return nil, action_error("id is required", errors.INVALID)
    end
    if type(method_name) ~= "string" or method_name == "" then
        return nil, action_error("method is required", errors.INVALID)
    end
    if M.is_lifecycle(method_name) then
        return nil, action_error("lifecycle method '" .. method_name .. "' is not callable as an action", errors.INVALID)
    end
    local registry = mod("registry") :: automation_types.RegistryModule?
    local component = mod("component") :: automation_types.ComponentModule?
    if not registry or not component then
        return nil, action_error("platform modules unavailable", errors.UNAVAILABLE)
    end

    local required_access = action_required_access(component, method_name)
    local component_rows, access_err = component.query({
        component_ids = { component_id },
        access_mask = required_access,
        limit = 1,
    })
    -- A failed read and an absent row are different answers: reporting the first
    -- as the second declares an automation permanently gone on what may be a
    -- momentary database failure.
    if access_err then
        return nil, action_error("automation lookup failed: " .. tostring(access_err), errors.INTERNAL)
    end
    local component_row = component_rows and component_rows[1]
    if not component_row or not component_row.impl_id then
        return nil, action_error("automation not found", errors.NOT_FOUND)
    end

    local raw_entry, get_err = registry.get(component_row.impl_id :: string)
    local entry = raw_entry :: automation_types.RegistryEntry?
    if get_err then
        return nil, action_error("binding lookup failed: " .. tostring(get_err), errors.INTERNAL)
    end
    if not entry then
        return nil, action_error("binding not found: " .. tostring(component_row.impl_id), errors.NOT_FOUND)
    end
    if entry.kind ~= "contract.binding" or type(entry.meta) ~= "table"
        or (entry.meta.type ~= AUTOMATION_TYPE and entry.meta.type ~= AUTOMATION_BINDING_META_TYPE) then
        return nil, action_error("component is not an automation", errors.INVALID)
    end
    local action = M.resolve_action(entry, method_name)
    if not action then
        return nil, action_error("method '" .. method_name .. "' is not declared as a public automation action", errors.INVALID)
    end

    local instance, open_err = component.open(component_id, required_access, action.contract)
    if open_err or not instance then
        return nil, preserve_kind(open_err, "open failed: " .. tostring(open_err))
    end
    local fn = instance[method_name]
    if type(fn) ~= "function" then
        return nil, action_error("method '" .. method_name .. "' could not be resolved on instance", errors.INVALID)
    end
    local result, call_err = fn(instance, type(args) == "table" and args or {})
    if call_err ~= nil then return nil, call_err end
    return ({ id = component_id, method = method_name, result = result }) :: automation_types.CallActionResult, nil
end

return M

