local contract = require("contract")
local component = require("component")
local persist = require("channel_persist")
local consts = require("channel_consts")
local types = require("channel_types")
local trace_context = require("trace_context")
local agent_ref = require("agent_ref")
local logger = require("logger"):named("kickside.channel.receive")
local run_as = require("channel_run_as")
local thread_bridge = require("channel_thread_bridge")

-- kickside.channel:receive -- the provider-neutral inbound seam for shared
-- channels, and the shared inbound-routing surface every conversation surface runs
-- on. A connection provider normalizes a channel message and delivers it here.
-- This module owns channel policy: it resolves WHICH installed responder
-- automation covers the channel (an indexed component-meta lookup keyed by
-- connection + channel), picks the agent + the frozen installer identity to run as,
-- and hands the turn to route_turn. Unrouted channels (no bot attached) are ignored,
-- not errors. No link/clear: a channel runs under the installer, not a linked
-- external user.
--
-- route_turn (plus normalize / trace_for_turn / send_hub / trim) is the single
-- inbound-routing implementation; kickside.dm imports it and drives it with a
-- subject-identity spec, so the session + trace + Route + hub-send orchestration
-- lives here once. Callers differ only in the spec they build and the extra result
-- fields they stamp (dm adds user_id).
local M = {}
M._process = process

local RUNTIME_READER = "kickside.contract:runtime_reader"
-- The automation binding installed responders register under; the routing lookup
-- scopes to it so it never scans other automation types.
local RESPONDER_IMPL = "kickside.channel.responder:channel_responder"
local DEFAULT_SESSION_IDLE_SECONDS = consts.DEFAULT_SESSION_IDLE_SECONDS
local DEFAULT_RUNTIME_IDLE_SECONDS = consts.DEFAULT_RUNTIME_IDLE_SECONDS

function M.trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function normalize_session_policy(value: any): { [string]: any }
    local raw = type(value) == "table" and (value :: { [string]: any }) or {}
    local out: { [string]: any } = {
        rotate_after_idle = raw.rotate_after_idle ~= false,
    }
    if out.rotate_after_idle == true then
        local idle = tonumber(raw.idle_seconds) or DEFAULT_SESSION_IDLE_SECONDS
        if idle < 60 then idle = 60 end
        if idle > 30 * 24 * 60 * 60 then idle = 30 * 24 * 60 * 60 end
        out.idle_seconds = math.floor(idle)
    end
    local runtime_idle = tonumber(raw.runtime_idle_seconds) or DEFAULT_RUNTIME_IDLE_SECONDS
    if runtime_idle < 60 then runtime_idle = 60 end
    if runtime_idle > 24 * 60 * 60 then runtime_idle = 24 * 60 * 60 end
    out.runtime_idle_seconds = math.floor(runtime_idle)
    return out
end

-- normalize copies the transport fields off a raw inbound and validates them.
-- opts.require_external_user additionally requires a non-empty external_user_id
-- (the DM surface routes by external identity; a shared channel does not).
function M.normalize(args: any, opts: any?): (any?, string?)
    opts = type(opts) == "table" and opts or {}
    args = type(args) == "table" and args or {}
    local inbound: { [string]: any } = {}
    inbound.provider = M.trim(args.provider)
    inbound.reply_component_id = M.trim(args.reply_component_id)
    inbound.reply_resource_id = M.trim(args.reply_resource_id or args.channel_id)
    inbound.channel_id = M.trim(args.channel_id)
    inbound.external_user_id = M.trim(args.external_user_id)
    inbound.external_username = M.trim(args.external_username)
    inbound.external_display_name = M.trim(args.external_display_name)
    inbound.external_message_id = M.trim(args.external_message_id)
    inbound.text = type(args.text) == "string" and args.text or ""
    inbound.attachments = type(args.attachments) == "table" and args.attachments or {}
    inbound.trace_context = type(args.trace_context) == "table" and args.trace_context or nil

    if inbound.provider == "" then return nil, "provider is required" end
    if inbound.reply_resource_id == "" then return nil, "reply_resource_id is required" end
    if inbound.channel_id == "" then return nil, "channel_id is required" end
    if opts.require_external_user and inbound.external_user_id == "" then return nil, "external_user_id is required" end
    if inbound.text == "" and #inbound.attachments == 0 then return nil, "text or attachments are required" end
    return inbound, nil
end

-- trace_for_turn derives the turn's trace context. domain is the correlation kind
-- ("channel" | "dm"); extra_parts are correlation parts inserted after the
-- {provider, reply_component_id, channel_id} prefix and before the optional
-- {"message", message_id} suffix (dm inserts {"user", external_id}). An inbound
-- trace context is continued (correlation key filled if missing); otherwise a fresh
-- one begins.
function M.trace_for_turn(inbound: any, domain: string, extra_parts: { string }): (any?, string?)
    local parts: { any } = {
        M.trim((inbound :: any).provider),
        M.trim((inbound :: any).reply_component_id),
        M.trim((inbound :: any).channel_id),
    }
    if type(extra_parts) == "table" then
        for _, part in ipairs(extra_parts :: { any }) do
            parts[#parts + 1] = part
        end
    end
    local message_id = M.trim((inbound :: any).external_message_id)
    if message_id ~= "" then
        parts[#parts + 1] = "message"
        parts[#parts + 1] = message_id
    end
    local correlation_key, corr_err = trace_context.pack_correlation(domain, parts)
    if corr_err then return nil, corr_err end

    if type((inbound :: any).trace_context) == "table" then
        local ctx, ctx_err = trace_context.normalize((inbound :: any).trace_context)
        if ctx_err then return nil, ctx_err end
        if ctx then
            if not (ctx :: table).correlation_key then
                (ctx :: table).correlation_key = correlation_key
            end
            return ctx, nil
        end
    end
    return trace_context.begin(domain, correlation_key), nil
end

function M.send_hub(payload: any): (boolean, string?)
    local proc: any = M._process
    local ok, send_err = proc.send(consts.HUB_NAME, consts.HUB_TOPIC, payload)
    if not ok then return false, tostring(send_err or "hub unavailable") end
    return true, nil
end

-- route_turn is the shared session + trace + Route + hub-send orchestrator. The
-- caller resolves identity + policy and hands a spec: the route key, the session
-- (provider, subject) to map, run_as, the thread shape, session start config, the
-- reply event pair, and the trace domain + extra correlation parts. It resolves the
-- conversation session id, derives the trace, assembles the Route (reply target
-- trimmed off inbound), and sends the turn to the hub. Returns the routed/error
-- result; the caller stamps any extra fields (dm adds user_id).
function M.route_turn(inbound: any, spec: types.RouteSpec): any
    local db_id, derr = consts.db_id()
    if derr or not db_id then
        return { accepted = false, status = "error", error = tostring(derr) }
    end
    local policy = type((spec :: any).session_policy) == "table" and (spec :: any).session_policy or {}
    local max_idle_seconds: integer? = nil
    if (policy :: any).rotate_after_idle == true then
        local raw_idle = tonumber((policy :: any).idle_seconds)
        if raw_idle and raw_idle > 0 then max_idle_seconds = math.floor(raw_idle) end
    end
    local session_id, _, serr = persist.get_or_create(db_id, consts.SESSION_MAP_TABLE, spec.session_provider, spec.session_subject, max_idle_seconds)
    if serr or not session_id or session_id == "" then
        return { accepted = false, status = "error", error = "could not resolve conversation session: " .. tostring(serr) }
    end

    local turn_trace, trace_err = M.trace_for_turn(inbound, spec.trace_domain, spec.trace_parts)
    if trace_err then
        return { accepted = false, status = "error", session_id = session_id, error = trace_err }
    end
    (inbound :: any).trace_context = turn_trace

    local route: types.Route = {
        key = spec.route_key,
        session_id = session_id,
        run_as = spec.run_as,
        thread = spec.thread,
        start = spec.start,
        reply = {
            component_id = M.trim((inbound :: any).reply_component_id),
            resource_id = M.trim((inbound :: any).reply_resource_id),
            sent_event = spec.reply_sent_event,
            failed_event = spec.reply_failed_event,
        },
        inbound = inbound :: types.Envelope,
    }
    local sent, send_err = M.send_hub({ kind = "turn", route = route })
    if not sent then
        return {
            accepted = false,
            status = "error",
            session_id = session_id,
            error = "could not route conversation turn to hub: " .. tostring(send_err or "hub unavailable"),
        }
    end
    return { accepted = true, status = "routed", session_id = session_id }
end

-- resolve_route(component_id, channel_id) -> (matched?, err?). Finds the responder
-- covering this channel on this connection through ONE indexed read: the routing
-- keys live in component meta (kickside_component_meta, composite key/value index),
-- so a with_meta lookup selects the responder id directly -- no list-everything scan.
-- The single matched component's private state is then read for the authoritative
-- pause flag and the routing overlay, and its engine-frozen execution identity to
-- run as. Unrouted (no responder) and paused both return no match; the caller treats
-- either as unrouted. A trusted system read (opened without an actor).
local function resolve_route(component_id: string, channel_id: string): (any?, string?)
    local rows = component.list_system({
        impl_ids = { RESPONDER_IMPL },
        meta = { connection_id = component_id, channel_id = channel_id },
    })
    if type(rows) ~= "table" or #(rows :: { any }) == 0 then
        return nil, "no responder covers this channel"
    end
    local matched_id = M.trim((rows :: { any })[1].component_id)
    if matched_id == "" then return nil, "no responder covers this channel" end

    local def, gerr = contract.get(RUNTIME_READER)
    if gerr or not def then return nil, "runtime_reader unavailable: " .. tostring(gerr) end
    local reader, oerr = (def :: any):open()
    if oerr or not reader then return nil, "runtime_reader open failed: " .. tostring(oerr) end
    local res, rerr = reader:get({ id = matched_id, include_state = true })
    if rerr then return nil, tostring(rerr) end
    if type(res) ~= "table" or (res :: any).success == false then
        return nil, tostring(res and (res :: any).error or "runtime_reader get failed")
    end
    local automation = type((res :: any).automation) == "table" and (res :: any).automation or {}
    local state = type((automation :: any).state) == "table" and (automation :: any).state or {}
    -- Pause is authoritative from private state, not projected meta, so it takes
    -- effect on the very next turn. A paused responder still resolves (identity +
    -- paused flag): the turn does not route, but the inbound is witnessed on the
    -- channel thread under the installer identity so the channel record stays
    -- complete regardless of routing.
    return {
        component_id = matched_id,
        paused = (state :: any).paused == true,
        agent_id = M.trim((state :: any).agent_id),
        identity = (automation :: any).execution_identity,
        traits = type((state :: any).traits) == "table" and (state :: any).traits or {},
        trait_contexts = type((state :: any).trait_contexts) == "table" and (state :: any).trait_contexts or {},
        context = M.trim((state :: any).context),
        activation_context = type((state :: any).activation_context) == "table" and (state :: any).activation_context or {},
        response_policy = type((state :: any).response_policy) == "table" and (state :: any).response_policy or {},
        session_policy = normalize_session_policy((state :: any).session_policy),
        system_prompt = M.trim((state :: any).system_prompt),
    }, nil
end

-- agent_exists reports whether the responder's frozen agent still exists. A deleted
-- agent must fail the turn visibly here, not crash the session runtime downstream.
local function agent_exists(agent_id: string): boolean
    if agent_id == "" then return false end
    local component_id = agent_ref.user_component_id(agent_id)
    if component_id == nil then return true end
    local rows = component.list_system({ component_ids = { component_id } })
    return type(rows) == "table" and #(rows :: { any }) > 0
end

-- _witness_inbound records an inbound message on the channel's audit thread under
-- the responder's frozen installer identity. The ingress function holds no ambient
-- identity, so it reconstructs the installer frame and authors the write through the
-- shared thread bridge (whose append is idempotent on the external message id). It is
-- best-effort: a witness failure is logged and never changes the routing result.
-- A seam so tests assert the witness without the identity/threads runtime.
function M._witness_inbound(inbound: any, identity: any)
    -- A witness failure is best-effort at ingress: it is logged with the channel
    -- coordinates so an operator can locate a persistently failing channel, and it
    -- never changes the routing result or crashes receive. run_as.resolve failing
    -- (a deleted installer principal) is a truthful skip, never a system-identity
    -- fallback. NOTE: a persistent witness failure is not yet surfaced on the
    -- responder/channel status (a listening automation would see silent gaps); see
    -- the report -- that observable path needs a responder witness-health field it
    -- does not declare today.
    local where = {
        provider = M.trim((inbound :: any).provider),
        channel_id = M.trim((inbound :: any).channel_id),
        reply_component_id = M.trim((inbound :: any).reply_component_id),
    }
    local actor, scope, rerr = run_as.resolve({ mode = consts.RUN_AS.FROZEN, identity = identity })
    if rerr or not actor or not scope then
        logger:warn("channel witness identity unavailable", {
            error = tostring(rerr),
            provider = where.provider,
            channel_id = where.channel_id,
            reply_component_id = where.reply_component_id,
        })
        return
    end
    local thread_id, _, terr = thread_bridge.ensure_thread(inbound, {
        thread_class = consts.THREAD_CLASS,
        title_prefix = "Channel",
        managed_by = "kickside.channel",
        receive_target = "kickside.channel:receive",
        role = "channel",
        user_ids = { M.trim((identity :: any).actor_id) },
    }, actor, scope)
    if terr or not thread_id then
        logger:warn("channel witness ensure thread failed", {
            error = tostring(terr),
            provider = where.provider,
            channel_id = where.channel_id,
            reply_component_id = where.reply_component_id,
        })
        return
    end
    local _, aerr = thread_bridge.append_inbound(thread_id, inbound, {
        event_type = consts.EVENT.INBOUND,
        external_source = consts.THREAD_CLASS .. "." .. M.trim((inbound :: any).provider),
    }, actor, scope)
    if aerr then
        logger:warn("channel witness append failed", {
            error = tostring(aerr),
            thread_id = thread_id,
            provider = where.provider,
            channel_id = where.channel_id,
            reply_component_id = where.reply_component_id,
        })
    end
end

function M.receive(args: any): any
    local inbound, nerr = M.normalize(args)
    if nerr or not inbound then
        return { accepted = false, status = "error", error = nerr or "invalid inbound" }
    end
    local provider: string = M.trim(inbound.provider)
    local component_id: string = M.trim(inbound.reply_component_id)
    local channel_id: string = M.trim(inbound.channel_id)

    -- 1. Route: which responder covers this channel, and as whom to run.
    local matched, rerr = resolve_route(component_id, channel_id)
    local identity = matched and (matched :: any).identity or nil
    if rerr or type(matched) ~= "table" or type(identity) ~= "table" then
        -- No responder covers this channel: no owner identity exists at ingress, so
        -- the message is not witnessed here (the documented boundary).
        return { accepted = false, status = "unrouted", error = rerr or "no responder" }
    end
    local agent_id = M.trim((matched :: any).agent_id)
    local paused = (matched :: any).paused == true
    local agent_ok = agent_id ~= "" and agent_exists(agent_id)

    -- 2. Witness the inbound whenever routing declines the turn (paused responder, or
    -- an agent deleted since install). The active turn is witnessed by the bridge, so
    -- witnessing here covers exactly the messages the bridge never sees -- one witness
    -- per message on a responder-covered channel, regardless of routing.
    if paused or not agent_ok then
        M._witness_inbound(inbound, identity)
        if paused then
            return { accepted = false, status = "unrouted", error = "responder paused" }
        end
        if agent_id == "" then
            return { accepted = false, status = "unrouted", error = "no responder" }
        end
        -- The responder's agent is frozen at install; if it was deleted since, fail the
        -- turn visibly instead of routing to a dead agent and crashing downstream.
        return { accepted = false, status = "error", error = "responder agent " .. agent_id .. " no longer exists" }
    end
    local user_id = M.trim((identity :: any).actor_id)

    -- 2. Build the channel spec (frozen installer identity, channel consts,
    -- per-responder system_prompt) and route it through the shared orchestrator.
    local spec: types.RouteSpec = {
        route_key = consts.external_ref(provider, component_id, channel_id),
        session_provider = provider,
        session_subject = channel_id,
        run_as = { mode = consts.RUN_AS.FROZEN, identity = identity },
        thread = {
            class = consts.THREAD_CLASS,
            title_prefix = "Channel",
            managed_by = "kickside.channel",
            receive_target = "kickside.channel:receive",
            role = "channel",
            user_ids = { user_id },
            inbound_event = consts.EVENT.INBOUND,
        },
        start = {
            component_id = (matched :: any).component_id,
            agent_id = agent_id,
            session_kind = "CHANNEL",
            system_prompt = (matched :: any).system_prompt :: string,
            traits = (matched :: any).traits :: { string },
            trait_contexts = (matched :: any).trait_contexts,
            context = (matched :: any).context :: string,
            activation_context = (matched :: any).activation_context,
            response_policy = (matched :: any).response_policy,
            session_policy = (matched :: any).session_policy,
        },
        session_policy = (matched :: any).session_policy,
        reply_sent_event = consts.EVENT.REPLY_SENT,
        reply_failed_event = consts.EVENT.REPLY_FAILED,
        trace_domain = "channel",
        trace_parts = {},
    }
    return M.route_turn(inbound, spec)
end

return M

