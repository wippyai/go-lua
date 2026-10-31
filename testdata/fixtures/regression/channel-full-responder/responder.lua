-- Generic channel-responder automation type: "when a message lands in this
-- channel, run this agent as me". Provider-neutral -- every provider's responder
-- binding (Discord, Slack, ...) points install/delete here; the binding only adds
-- provider-specific meta (icon, provider, inputs schema). The
-- automation ENGINE freezes the installer's execution identity onto the row;
-- kickside.channel:receive reads it back via runtime_reader at the execution
-- boundary. The state shape (connection_id, channel_id, agent_id, paused) is what
-- receive's routing matches on. No external resources, so delete is a no-op.
local automations_lib = require("automations_lib")
local component = require("component")
local ctx = require("ctx")
local contract = require("contract")
local security = require("security")
local consts = require("channel_consts")

local M = {}

type Map = { [string]: any }

-- The shared conversation runtime's teardown step. Registered as this responder's
-- install rollback so uninstall (explicit or cascaded from a connection/user
-- deletion) drops the conversation session it owns.
local CLEANUP_TARGET = "kickside.channel.binding:cleanup_conversation"
local THREADS_CONTRACT = "kickside.core:threads"
local RESPONDER_IMPL = "kickside.channel.responder:channel_responder"
local STATUS_CHANGED = "kickside.channel.responder.events:status.changed"

-- Per-responder conversation instruction the channel runtime applies at session
-- start. Configurable per responder (form field pending); this is the default when
-- an install omits one, so the transport no longer hardcodes it. Provider-neutral.
local DEFAULT_SYSTEM_PROMPT = "You are responding in a channel. Keep responses concise and direct. No emojis. If you need external information, use available capabilities or tools before answering."

type ResponderState = {
    connection_id: string,
    kind: string,
    channel_id: string,
    channel_name: string,
    agent_id: string,
    agent_name: string,
    title: string,
    paused: boolean,
    traits: { string },
    trait_contexts: Map,
    context: string,
    activation_context: Map,
    response_policy: Map,
    session_policy: Map,
    system_prompt: string,
}

local MAX_TRAITS = 40
local DEFAULT_LAZY_WAIT_SECONDS = 8
local DEFAULT_ASSESSOR_MODEL = "class:fast"
local DEFAULT_SESSION_IDLE_SECONDS = consts.DEFAULT_SESSION_IDLE_SECONDS
local DEFAULT_RUNTIME_IDLE_SECONDS = consts.DEFAULT_RUNTIME_IDLE_SECONDS

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function component_id(args: any): (string?, string?)
    local from_args = type(args) == "table" and (args :: Map).component_id or nil
    if type(from_args) == "string" and trim(from_args) ~= "" then return trim(from_args), nil end
    local id = ctx.get("component_id")
    if type(id) == "string" and trim(id) ~= "" then return trim(id), nil end
    return nil, "component_id not in scope"
end

-- Dedup a list of trait ids, dropping blanks, capped so a runaway input can't bloat
-- the row. Mirrors the agent invoke endpoint's trait handling.
local function string_list(value: any): { string }
    local out: { string } = {}
    local seen: { [string]: boolean } = {}
    if type(value) ~= "table" then return out end
    for _, v in ipairs(value :: { any }) do
        local id = trim(v)
        if id ~= "" and not seen[id] then
            seen[id] = true
            out[#out + 1] = id
            if #out >= MAX_TRAITS then break end
        end
    end
    return out
end

-- Per-trait context map, keeping only entries whose key is a selected trait and
-- whose value is an object (the trait's web_component config).
local function copy_contexts(value: any, selected: { string }): Map
    local out: Map = {}
    if type(value) ~= "table" then return out end
    local allowed: { [string]: boolean } = {}
    for _, id in ipairs(selected) do allowed[id] = true end
    for id, ctx in pairs(value :: Map) do
        if type(id) == "string" and allowed[id] and type(ctx) == "table" then out[id] = ctx end
    end
    return out
end

local function normalize_response_policy(value: any): Map
    local raw = type(value) == "table" and (value :: Map) or {}
    local mode = trim(raw.mode)
    if mode ~= "lazy" then mode = "immediate" end

    local out: Map = { mode = mode }
    if mode == "lazy" then
        local wait = tonumber(raw.wait_seconds) or DEFAULT_LAZY_WAIT_SECONDS
        if wait < 1 then wait = 1 end
        if wait > 120 then wait = 120 end
        out.wait_seconds = math.floor(wait)
        out.prompt = trim(raw.prompt)
        local model = trim(raw.assessor_model)
        out.assessor_model = model ~= "" and model or DEFAULT_ASSESSOR_MODEL
    end
    return out
end

local function normalize_session_policy(value: any): Map
    local raw = type(value) == "table" and (value :: Map) or {}
    local enabled = raw.rotate_after_idle ~= false
    local runtime_idle = tonumber(raw.runtime_idle_seconds) or DEFAULT_RUNTIME_IDLE_SECONDS
    if runtime_idle < 60 then runtime_idle = 60 end
    if runtime_idle > 24 * 60 * 60 then runtime_idle = 24 * 60 * 60 end
    local out: Map = {
        rotate_after_idle = enabled,
        runtime_idle_seconds = math.floor(runtime_idle),
    }
    if enabled then
        local idle = tonumber(raw.idle_seconds) or DEFAULT_SESSION_IDLE_SECONDS
        if idle < 60 then idle = 60 end
        if idle > 30 * 24 * 60 * 60 then idle = 30 * 24 * 60 * 60 end
        out.idle_seconds = math.floor(idle)
    end
    return out
end

local function normalize_input(input: any): (ResponderState?, Map?, string?)
    if type(input) ~= "table" then return nil, nil, "input must be an object" end
    local i = input :: Map
    local connection_id = trim(i.connection_id)
    if connection_id == "" then return nil, nil, "connection_id is required" end
    local agent_id = trim(i.agent_id)
    if agent_id == "" then return nil, nil, "agent_id is required" end
    local kind = trim(i.target or i.kind)
    if kind ~= "channel" then return nil, nil, "target must be channel" end
    local channel_id = trim(i.channel_id)
    if channel_id == "" then return nil, nil, "channel_id is required for a channel responder" end

    -- Display-only names captured from the install pickers; routing still keys on
    -- the ids. May be empty (older callers / re-installs), then ids are shown.
    local channel_name = trim(i.channel_name)
    local agent_name = trim(i.agent_name)

    local title = trim(i.title)
    if title == "" then
        title = (channel_name ~= "" and channel_name or ("Channel " .. channel_id)) .. " responder"
    end

    local traits = string_list(i.traits or i.additional_traits)
    local trait_contexts = copy_contexts(i.trait_contexts, traits)
    local context = trim(i.context)
    local activation_context: Map = {}
    if type(i.activation_context) == "table" then
        local raw = i.activation_context :: Map
        activation_context.enabled = raw.enabled == true
        local limit = tonumber(raw.limit)
        if limit and limit > 0 then activation_context.limit = math.floor(limit) end
    end
    local response_policy = normalize_response_policy(i.response_policy)
    local session_policy = normalize_session_policy(i.session_policy)
    local system_prompt = trim(i.system_prompt)
    if system_prompt == "" then system_prompt = DEFAULT_SYSTEM_PROMPT end

    local state: ResponderState = {
        connection_id = connection_id,
        kind = kind,
        channel_id = channel_id,
        channel_name = channel_name,
        agent_id = agent_id,
        agent_name = agent_name,
        title = title,
        paused = false,
        traits = traits,
        trait_contexts = trait_contexts,
        context = context,
        activation_context = activation_context,
        response_policy = response_policy,
        session_policy = session_policy,
        system_prompt = system_prompt,
    }
    -- connection_id/channel_id are promoted to public_state so the engine writes them
    -- into indexed component meta; kickside.channel:receive resolves the responder by a
    -- single with_meta lookup instead of scanning every installed automation.
    local public_state: Map = {
        status = "active",
        channel = channel_name ~= "" and channel_name or channel_id,
        agent = agent_name ~= "" and agent_name or agent_id,
        trait_count = tostring(#traits),
        connection_id = connection_id,
        channel_id = channel_id,
    }
    return state, public_state, nil
end

local function public_state_for(state: ResponderState): Map
    return {
        status = state.paused and "paused" or "active",
        channel = state.channel_name ~= "" and state.channel_name or state.channel_id,
        agent = state.agent_name ~= "" and state.agent_name or state.agent_id,
        trait_count = tostring(#state.traits),
        connection_id = state.connection_id,
        channel_id = state.channel_id,
    }
end

local function metadata_for(state: ResponderState): Map
    return {
        title = state.title,
        comment = "Replies in " .. (state.channel_name ~= "" and state.channel_name or state.channel_id)
            .. " as " .. (state.agent_name ~= "" and state.agent_name or state.agent_id),
    }
end

-- Resolve a connection component's provider from its meta. Captured at install so
-- the cleanup rollback works even after the connection itself is deleted.
local function provider_of(connection_id: string): string
    local rows = component.list_system({ component_ids = { connection_id }, include = { meta = true } })
    if type(rows) == "table" and (rows :: { any })[1] then
        local meta = ((rows :: { any })[1] :: any).meta
        if type(meta) == "table" then return trim((meta :: any).provider) end
    end
    return ""
end

-- existing_responder returns the display name of a responder already covering this
-- (connection, channel), or "" when none does. The same indexed meta lookup routing
-- uses, so a duplicate is rejected at install rather than racing at receive.
local function existing_responder(connection_id: string, channel_id: string): string
    local rows = component.list_system({
        impl_ids = { RESPONDER_IMPL },
        meta = { connection_id = connection_id, channel_id = channel_id },
        include = { meta = true },
    })
    if type(rows) ~= "table" or (rows :: { any })[1] == nil then return "" end
    local meta = ((rows :: { any })[1] :: any).meta
    local name = type(meta) == "table" and trim((meta :: any).title) or ""
    if name ~= "" then return name end
    return trim(((rows :: { any })[1] :: any).component_id)
end

-- install(input) -> ({ state, public_state, metadata, rollback }, nil) | (nil, err).
-- state is provider routing only; the engine adds the frozen execution identity. The
-- rollback chain carries one step -- delete the conversation session this responder owns
-- -- so uninstall (or a cascaded connection/user deletion) leaves no orphan session.
function M.install(input: any): (any, any)
    local state, public_state, input_err = normalize_input(input)
    if input_err then return nil, input_err end
    local s = state :: ResponderState

    -- One responder per (connection, channel): a second silently races routing (first
    -- by list order wins). Reject here, naming the incumbent. The component row for THIS
    -- responder is registered only after install returns, so it cannot match itself; two
    -- concurrent installs still race the check, an accepted small window.
    local incumbent = existing_responder(s.connection_id, s.channel_id)
    if incumbent ~= "" then
        return nil, "channel " .. s.channel_id .. " is already covered by responder '" .. incumbent .. "'"
    end

    local result: Map = {
        state = s,
        public_state = public_state,
        metadata = metadata_for(s),
    }
    -- Resolve the connection's provider now, while it still exists, so the cleanup
    -- rollback works even after the connection is deleted. Fail closed: a responder
    -- with no resolvable provider has no teardown and would orphan its session.
    local provider = provider_of(s.connection_id)
    if provider == "" then return nil, "connection " .. s.connection_id .. " has no provider; cannot install" end
    result.rollback = {
        { target = CLEANUP_TARGET, args = { provider = provider, external_id = s.channel_id } },
    }
    return result, nil
end

-- delete() -> deletable.delete handler (the post-uninstall component unregister
-- hook). The conversation session this responder owns is torn down by the install
-- rollback step (CLEANUP_TARGET), which the engine replays before this runs.
function M.delete(_args: any): any
    return { success = true }
end

function M.status(args: any): (any, any)
    local id, id_err = component_id(args)
    if not id then return nil, id_err end
    local state, err = automations_lib.read_public_state(id)
    if err then return nil, err end
    return state or {}, nil
end

function M.read_config(args: any): (any, any)
    local id, id_err = component_id(args)
    if not id then return nil, id_err end
    local state, err = automations_lib.read_state(id)
    if err then return nil, err end
    local public_state, public_err = automations_lib.read_public_state(id)
    if public_err then return nil, public_err end
    return { success = true, state = state or {}, public_state = public_state or {} }, nil
end

function M.configure(args: any): (any, any)
    local id, id_err = component_id(args)
    if not id then return nil, id_err end

    local current, read_err = automations_lib.read_state(id)
    if read_err then return nil, read_err end
    if type(current) ~= "table" then return nil, "responder state is missing" end
    local cur = current :: Map
    local input = type(args) == "table" and ((args :: Map).input or args) or {}
    if type(input) ~= "table" then return nil, "input must be an object" end
    local i = input :: Map

    -- The source channel owns rollback cleanup. Editing it is a resource move, not
    -- a state patch, so the manage action keeps routing fixed and edits behavior.
    local merged: Map = {}
    for k, v in pairs(i) do merged[k] = v end
    merged.connection_id = cur.connection_id
    merged.channel_id = cur.channel_id
    merged.channel_name = cur.channel_name
    merged.kind = "channel"
    merged.target = "channel"

    local state, _, input_err = normalize_input(merged)
    if input_err then return nil, input_err end
    local s = state :: ResponderState
    s.paused = cur.paused == true

    local _, private_err = automations_lib.patch_state(id, s)
    if private_err then return nil, private_err end

    local public_state = public_state_for(s)
    local meta = metadata_for(s)
    for k, v in pairs(public_state) do meta[k] = v end
    local ok, meta_err = component.set_meta(id, meta)
    if not ok then return nil, meta_err or "component.set_meta failed" end

    return { success = true, state = s, public_state = public_state }, nil
end

local function emit_event(component_id: string, etype: string, body: Map): string?
    local actor = security.actor()
    if not actor then return "security actor is required to emit responder event" end
    local scope = security.scope()
    if not scope then return "security scope is required to emit responder event" end

    local def, gerr = contract.get(THREADS_CONTRACT)
    if gerr or not def then return "threads contract unavailable: " .. tostring(gerr) end
    local inst, oerr = (def :: any):with_actor(actor):with_scope(scope):open()
    if oerr or not inst then return "threads open failed: " .. tostring(oerr) end
    local _, err = inst:emit({
        component_id = component_id,
        type = etype,
        body = body,
        impl_id = RESPONDER_IMPL,
        thread_class = consts.RESPONDER_THREAD_CLASS,
    })
    if err then return tostring(err) end
    return nil
end
M._emit_event = emit_event

local function set_paused(args: any, paused: boolean): (any, any)
    local id, id_err = component_id(args)
    if not id then return nil, id_err end

    local _, private_err = automations_lib.patch_state(id, { paused = paused })
    if private_err then return nil, private_err end

    local status = paused and "paused" or "active"
    local emit_err = M._emit_event(id, STATUS_CHANGED, { paused = paused, status = status })
    if emit_err then return nil, emit_err end

    return { success = true, status = status, paused = paused }, nil
end

function M.pause(args: any): (any, any)
    return set_paused(args, true)
end

function M.resume(args: any): (any, any)
    return set_paused(args, false)
end

return M

