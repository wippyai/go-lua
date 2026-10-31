local security = require("security")
local funcs = require("funcs")
local json = require("json")
local contract = require("contract")

-- Canonical frozen execution identity: one capture/reconstruct/run-as primitive
-- shared by every engine that runs deferred work as the actor who authorized it
-- (the scheduler, projections, cron tasks, installed automations, channel session
-- triggers). The frozen record is the authority snapshot -- a durable actor-claim
-- subset plus the reconstructable scope name resolved by the users contract.
-- Reconstruction is fail-closed: a missing/revoked scope or a non-active actor
-- does not run. Lives in core execution so engines never reinvent it or pull
-- component/lifecycle dependencies just to carry identity.
local M = {}

local VERSION = 1
M.VERSION = VERSION

-- Canonical private_context key the frozen identity is stored under on a component
-- (set at provision via PATCH_CONTEXT, read back by deferred workers to reconstruct).
-- One source of truth so no module hardcodes the string.
M.CONTEXT_KEY = "_execution_identity"

-- Canonical PERSISTED shape, used by every store project-wide (schedules,
-- projections, tokens, contexts): one indexable `actor_id` plus an extensible
-- `actor_context` JSON (the named scope + actor claims today; room for more later).
-- Tables expose these as two columns (actor_id indexable, actor_context TEXT/JSON);
-- JSON carriers (tokens, private_context) nest the same two fields. Serialize with
-- to_row, rebuild with from_row -- no module hand-rolls actor_id/scope/meta columns.
M.COLUMNS = { ID = "actor_id", CONTEXT = "actor_context" }

type Map = { [string]: any }

type ActorClaims = {
    user_id: string?,
    email: string?,
    full_name: string?,
    status: string?,
    security_groups: { string },
}

type ActorContext = {
    version: number,
    scope_id: string,
    claims: ActorClaims,
    captured_at: number?,
    captured_by: string?,
}

type FrozenIdentity = {
    actor_id: string,
    actor_context: ActorContext,
}

type ReconstructOptions = {
    live: boolean?,
    subject_id: string?,
    app_scope: string?,
    scope_resolver: any?,
}

-- Runtime seams, swapped in tests so capture/reconstruct is verified without a
-- live security registry.
M._security = security :: any
M._funcs = funcs :: any
M._contract = contract :: any

local function copy_string_array(values: any): { string }
    local out: { string } = {}
    if type(values) ~= "table" then return out end
    for _, value in ipairs(values :: { any }) do
        if type(value) == "string" and value ~= "" then out[#out + 1] = value end
    end
    return out
end

local SCOPE_RESOLVER = "kickside.contract:scope_resolver"

local function is_system_actor(actor: any, actor_id: string, meta: Map): boolean
    return meta.system == true or actor_id:sub(1, 7) == "system:"
end

local function resolve_scope_params(actor: any, actor_id: string, explicit_scope_name: any): (Map?, string?)
    local meta: Map = (actor:meta() or {}) :: Map
    if type(explicit_scope_name) == "string" and explicit_scope_name ~= "" then
        if not is_system_actor(actor, actor_id, meta) then
            return nil, "explicit execution scope is only valid for system actors"
        end
        return {
            subject_id = actor_id,
            groups = copy_string_array(meta.security_groups),
            scope_name = explicit_scope_name,
            meta = {
                user_id = actor_id,
                email = meta.email,
                full_name = meta.full_name,
                status = meta.status,
            },
        }, nil
    end

    if type(M._security.scope) ~= "function" then
        return nil, "security scope is unavailable"
    end
    local ambient_scope = M._security.scope()
    if not ambient_scope then
        return nil, "security scope is unavailable"
    end

    local resolver_contract, contract_err = M._contract.get(SCOPE_RESOLVER)
    if contract_err or not resolver_contract then
        return nil, "scope resolver unavailable: " .. tostring(contract_err)
    end
    local resolver, open_err = resolver_contract:with_actor(actor):with_scope(ambient_scope):open()
    if open_err or not resolver then
        return nil, "scope resolver open failed: " .. tostring(open_err)
    end
    -- Omit subject_id deliberately: capture freezes the ambient signed actor
    -- snapshot. Explicit subject_id is reserved for live users-store resolution
    -- by run-as callers such as cron/channel.
    local params, resolve_err = resolver:resolve({})
    if resolve_err or type(params) ~= "table" then
        return nil, "scope resolve failed: " .. tostring(resolve_err)
    end
    local scope_name = tostring((params :: Map).scope_name or "")
    if scope_name == "" then
        return nil, "scope resolver returned no scope"
    end
    return params :: Map, nil
end

-- capture(captured_by?) -> (FrozenIdentity?, err?). Freezes the current security
-- frame: actor id, a resolver-derived named scope, and a durable claim subset.
-- User actors resolve through kickside.contract:scope_resolver. System actors may
-- pass opts.scope_name from an explicit system call path. Runtime scope objects are
-- authorization frames only; their internals are never used as durable identity.
function M.capture(captured_by: string?, opts: any?): (FrozenIdentity?, string?)
    local actor = M._security.actor()
    local id = actor and tostring(actor:id()) or ""
    if not actor or id == "" then
        return nil, "an authenticated actor is required to capture an execution identity"
    end
    local meta: Map = (actor:meta() or {}) :: Map
    local resolved, scope_err = resolve_scope_params(actor, id, type(opts) == "table" and (opts :: Map).scope_name or nil)
    if not resolved then
        return nil, scope_err or "execution identity is not reconstructable"
    end
    local resolved_any = resolved :: any
    local scope_id = tostring(resolved_any["scope_name"] or "")
    local raw_resolved_meta = resolved_any["meta"]
    local resolved_meta: Map = {}
    if type(raw_resolved_meta) == "table" then
        resolved_meta = raw_resolved_meta :: Map
    end
    local resolved_groups = resolved_any["groups"]
    if type(resolved_groups) ~= "table" then
        return nil, "scope resolver returned no groups"
    end
    local identity: FrozenIdentity = {
        actor_id = id,
        actor_context = {
            version = VERSION,
            scope_id = scope_id,
            claims = {
                user_id = id,
                email = resolved_meta["email"] or meta["email"],
                full_name = resolved_meta["full_name"] or meta["full_name"],
                status = resolved_meta["status"] or meta["status"],
                security_groups = copy_string_array(resolved_groups),
            },
            captured_at = tonumber(os.time()) or 0,
            captured_by = captured_by,
        } :: ActorContext,
    }
    return identity, nil
end

-- validate(identity) -> (ok, err?). The schema invariant every consumer relies
-- on: supported version, present actor + scope, and durable claims.
function M.validate(identity: any): (boolean, string?)
    if type(identity) ~= "table" then return false, "identity must be a table" end
    local id = identity :: FrozenIdentity
    if type(id.actor_id) ~= "string" or id.actor_id == "" then return false, "missing actor_id" end
    local context = id.actor_context
    if type(context) ~= "table" then return false, "missing actor_context" end
    if (tonumber((context :: ActorContext).version) or 0) ~= VERSION then return false, "unsupported execution identity version" end
    if type((context :: ActorContext).scope_id) ~= "string" or (context :: ActorContext).scope_id == "" then
        return false, "missing actor_context.scope_id"
    end
    local claims = (context :: ActorContext).claims
    if type(claims) ~= "table" then return false, "missing actor_context.claims" end
    return true, nil
end

-- 'deleting' is the users module's tombstone: the account is dead from the moment
-- it is set, while its row survives until the deletion is reported and acknowledged.
-- A deferred identity must refuse it exactly as it refuses the settled states.
local TERMINAL_STATUS = { disabled = true, suspended = true, deleted = true, deleting = true, inactive = true }

M.LIVE_ERROR_KIND = {
    INFRASTRUCTURE = "infrastructure",
    SUBJECT = "subject",
}

local function terminal_status_error(status: any): string?
    local text = tostring(status or "")
    if text ~= "" and TERMINAL_STATUS[text:lower()] then
        return "actor status is " .. text .. "; refusing to reconstruct"
    end
    return nil
end

local function frozen_actor_and_scope(id: FrozenIdentity, check_terminal: boolean): (any?, any?, string?)
    local context = id.actor_context
    local claims = context.claims
    if check_terminal then
        local status_err = terminal_status_error((claims :: Map).status)
        if status_err then return nil, nil, status_err end
    end
    local actor = M._security.new_actor(id.actor_id, claims or {})
    if not actor then return nil, nil, "could not build actor " .. id.actor_id end
    local scope, scope_err = M._security.named_scope(context.scope_id)
    if scope_err or not scope then
        return nil, nil, "could not recover scope " .. context.scope_id .. ": " .. tostring(scope_err)
    end
    return actor, scope, nil
end

local function open_live_resolver(identity: FrozenIdentity, opts: ReconstructOptions): (any?, string?)
    local opener_actor, opener_scope, opener_err = frozen_actor_and_scope(identity, false)
    if opener_err then return nil, opener_err end

    local resolver_contract = opts.scope_resolver
    if not resolver_contract then
        local contract_err
        resolver_contract, contract_err = M._contract.get(SCOPE_RESOLVER)
        if contract_err or not resolver_contract then
            return nil, "scope resolver unavailable: " .. tostring(contract_err)
        end
    end

    local resolver, open_err = (resolver_contract :: any):with_actor(opener_actor):with_scope(opener_scope):open()
    if open_err or not resolver then
        return nil, "scope resolver open failed: " .. tostring(open_err)
    end
    return resolver, nil
end

local function live_subject_id(id: FrozenIdentity, opts: ReconstructOptions): string
    local explicit = type(opts.subject_id) == "string" and opts.subject_id or ""
    if explicit ~= "" then return explicit end
    return id.actor_id
end

local function has_live_scope(params: Map): boolean
    if type(params.scope_name) == "string" and params.scope_name ~= "" then return true end
    local ids = params.scope_policy_ids
    return type(ids) == "table" and #(ids :: { any }) > 0
end

local function actor_scope_from_live_params(params: Map): (any?, any?, string?, string?)
    local subject_id = tostring(params.subject_id or "")
    if subject_id == "" then
        return nil, nil, "scope resolver returned no subject", M.LIVE_ERROR_KIND.SUBJECT
    end
    local raw_meta = type(params.meta) == "table" and (params.meta :: Map) or {}
    local status_err = terminal_status_error(raw_meta.status)
    if status_err then return nil, nil, status_err, M.LIVE_ERROR_KIND.SUBJECT end

    local actor_meta: Map = {}
    for k, v in pairs(raw_meta) do actor_meta[k] = v end
    actor_meta.security_groups = copy_string_array(params.groups)

    local actor = M._security.new_actor(subject_id, actor_meta)
    if not actor then return nil, nil, "could not build actor " .. subject_id, M.LIVE_ERROR_KIND.SUBJECT end

    if type(params.scope_name) == "string" and params.scope_name ~= "" then
        local scope, scope_err = M._security.named_scope(params.scope_name)
        if scope_err or not scope then
            return nil, nil, "could not recover scope " .. tostring(params.scope_name) .. ": " .. tostring(scope_err), M.LIVE_ERROR_KIND.SUBJECT
        end
        return actor, scope, nil, nil
    end

    local scope_policy_ids = params.scope_policy_ids
    if type(scope_policy_ids) == "table" and #(scope_policy_ids :: { any }) > 0 then
        local policies: { any } = {}
        for i, policy_id in ipairs(scope_policy_ids :: { any }) do
            if type(policy_id) ~= "string" or policy_id == "" then
                return nil, nil, "scope resolver returned invalid policy id", M.LIVE_ERROR_KIND.SUBJECT
            end
            if type(M._security.policy) ~= "function" then
                return nil, nil, "security policy lookup is unavailable", M.LIVE_ERROR_KIND.INFRASTRUCTURE
            end
            local policy, policy_err = M._security.policy(policy_id)
            if policy_err or not policy then
                return nil, nil, "could not recover policy " .. policy_id .. ": " .. tostring(policy_err), M.LIVE_ERROR_KIND.INFRASTRUCTURE
            end
            policies[i] = policy
        end
        if type(M._security.new_scope) ~= "function" then
            return nil, nil, "security scope construction is unavailable", M.LIVE_ERROR_KIND.INFRASTRUCTURE
        end
        local scope, scope_err = M._security.new_scope(policies)
        if scope_err or not scope then
            return nil, nil, "could not recover live scope: " .. tostring(scope_err), M.LIVE_ERROR_KIND.INFRASTRUCTURE
        end
        return actor, scope, nil, nil
    end

    return nil, nil, "scope resolver returned no scope", M.LIVE_ERROR_KIND.SUBJECT
end

-- A frozen identity records the authentication epoch it was captured under. When
-- the resolver answers with a different one the credential behind this identity
-- has been revoked since, and the identity is refused. Both sides must carry an
-- epoch for the comparison to mean anything: an identity captured before epochs
-- existed keeps resolving, which is the same defined cutover the tokens have.
local function epoch_error(identity: FrozenIdentity, resolved_meta: Map): string?
    local context = identity.actor_context
    local claims = type(context) == "table" and (context :: any).claims or nil
    if type(claims) ~= "table" then return nil end
    local frozen_epoch = tonumber((claims :: Map).auth_epoch)
    local live_epoch = tonumber(resolved_meta.auth_epoch)
    if frozen_epoch == nil or live_epoch == nil then return nil end
    if frozen_epoch ~= live_epoch then
        return "actor authentication epoch is stale; refusing to reconstruct"
    end
    return nil
end

local function reconstruct_live(identity: FrozenIdentity, opts: ReconstructOptions): (any?, any?, string?, string?)
    local resolver, open_err = open_live_resolver(identity, opts)
    if open_err or not resolver then
        return nil, nil, open_err or "scope resolver unavailable", M.LIVE_ERROR_KIND.INFRASTRUCTURE
    end

    local requested_subject = live_subject_id(identity, opts)
    local params, resolve_err = resolver:resolve({
        subject_id = requested_subject,
        app_scope = opts.app_scope,
    })
    if resolve_err or type(params) ~= "table" then
        return nil, nil, "scope resolve failed: " .. tostring(resolve_err), M.LIVE_ERROR_KIND.INFRASTRUCTURE
    end

    local resolved = params :: Map
    local subject_error = resolved.error
    if subject_error ~= nil and subject_error ~= "" then
        return nil, nil, "run-as subject unresolved: " .. tostring(subject_error or requested_subject), M.LIVE_ERROR_KIND.SUBJECT
    end
    if not has_live_scope(resolved) then
        return nil, nil, "scope resolver returned no scope for " .. requested_subject, M.LIVE_ERROR_KIND.SUBJECT
    end

    if type(resolved.subject_id) ~= "string" or resolved.subject_id == "" then
        resolved.subject_id = requested_subject
    end

    local stale = epoch_error(identity, type(resolved.meta) == "table" and (resolved.meta :: Map) or {})
    if stale then return nil, nil, stale, M.LIVE_ERROR_KIND.SUBJECT end

    local actor, scope, build_err, build_kind = actor_scope_from_live_params(resolved)
    if build_err then
        return nil, nil, build_err, build_kind or M.LIVE_ERROR_KIND.SUBJECT
    end
    return actor, scope, nil, nil
end

-- reconstruct(identity, opts?) -> (actor?, scope?, err?, live_error_kind?).
-- Default mode rebuilds the actor from frozen claims and recovers the named
-- scope. opts.live=true re-fetches subject claims/groups/scope through the
-- canonical scope_resolver before building the runtime actor. Both modes are
-- fail-closed for terminal actor status; the live mode checks the fresh status.
function M.reconstruct(identity: any, opts: ReconstructOptions?): (any?, any?, string?, string?)
    local ok, verr = M.validate(identity)
    if not ok then return nil, nil, verr end
    local id = identity :: FrozenIdentity
    if type(opts) == "table" and opts.live == true then
        return reconstruct_live(id, opts :: ReconstructOptions)
    end
    local actor, scope, err = frozen_actor_and_scope(id, true)
    return actor, scope, err, nil
end

-- bind(identity) -> (funcs_builder?, err?). A funcs builder already scoped to the
-- reconstructed identity; the caller chains :call(fn_id, args).
function M.bind(identity: any, opts: ReconstructOptions?): (any?, string?)
    local actor, scope, err = M.reconstruct(identity, opts)
    if err then return nil, err end
    return M._funcs.new():with_actor(actor):with_scope(scope), nil
end

-- run_as(identity, fn_id, args, opts?) -> (result?, err?). Runs one function
-- under the reconstructed identity.
function M.run_as(identity: any, fn_id: string, args: any, opts: ReconstructOptions?): (any?, string?)
    local builder, err = M.bind(identity, opts)
    if not builder then return nil, err end
    local res, ferr = builder:call(fn_id, args)
    if ferr then return nil, tostring(ferr) end
    return res, nil
end

-- to_row(identity) -> ({ actor_id, actor_context }?, err?). Serialize to the
-- canonical persisted shape: actor_id (indexable) + actor_context (JSON holding
-- version, scope_id, the actor claims, and capture provenance). The single format
-- every store writes.
function M.to_row(identity: any): (any?, string?)
    local ok, verr = M.validate(identity)
    if not ok then return nil, verr end
    local id = identity :: FrozenIdentity
    local context, jerr = json.encode(id.actor_context)
    if jerr or not context then return nil, "failed to encode actor_context: " .. tostring(jerr) end
    return { actor_id = id.actor_id, actor_context = context }, nil
end

-- from_row(actor_id, actor_context) -> (FrozenIdentity?, err?). Rebuild from the
-- canonical columns. actor_context may be a JSON string or a decoded table.
function M.from_row(actor_id: any, actor_context: any): (FrozenIdentity?, string?)
    if type(actor_id) ~= "string" or actor_id == "" then
        return nil, "actor_id is required"
    end
    local ctx: Map = {}
    if type(actor_context) == "string" and actor_context ~= "" then
        local decoded, derr = json.decode(actor_context :: string)
        if derr or type(decoded) ~= "table" then return nil, "invalid actor_context json" end
        ctx = decoded :: Map
    elseif type(actor_context) == "table" then
        ctx = actor_context :: Map
    end
    local scope_id = tostring(ctx.scope_id or "")
    local raw_claims: Map = (type(ctx.claims) == "table" and (ctx.claims :: Map)) or {}
    local claims: ActorClaims = {
        user_id = raw_claims.user_id :: string?,
        email = raw_claims.email :: string?,
        full_name = raw_claims.full_name :: string?,
        status = raw_claims.status :: string?,
        security_groups = copy_string_array(raw_claims.security_groups),
    }
    local identity: FrozenIdentity = {
        actor_id = actor_id,
        actor_context = {
            version = tonumber(ctx.version) or VERSION,
            scope_id = scope_id,
            claims = claims :: any,
            captured_at = tonumber(ctx.captured_at),
            captured_by = ctx.captured_by :: string?,
        } :: ActorContext,
    }
    return identity, nil
end

-- reconstruct_row(actor_id, actor_context, opts?) -> (actor?, scope?, err?,
-- live_error_kind?). from_row + reconstruct -- the one call a store uses to turn
-- its persisted columns back into a bounded, fail-closed (actor, scope).
function M.reconstruct_row(actor_id: any, actor_context: any, opts: ReconstructOptions?): (any?, any?, string?, string?)
    local identity, err = M.from_row(actor_id, actor_context)
    if err or not identity then return nil, nil, err end
    return M.reconstruct(identity, opts)
end

return M

