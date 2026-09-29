local logger = require("logger"):named("kickside.channel.bridge")
local time = require("time")
local channel = require("channel")
local security = require("security")
local contract = require("contract")
local llm = require("llm")
local prompt = require("prompt")
local component = require("component")
local reply_sender = require("reply_sender")
local reply_policy = require("reply_policy")
local session_consts = require("session_consts")
local session_service = require("session_service")
local consts = require("channel_consts")
local thread_bridge = require("channel_thread_bridge")
local attachment_ingest = require("attachment_ingest")
local trace_context = require("trace_context")

local AGENT_RESOLVER = "wippy.agent:resolver"
local CHANNEL_CONTEXT_CONTRACT = "kickside.connection:channel_context"
local SESSION_ACTIVATED = "kickside.channel.responder.events:session.activated"
local SESSION_COMPLETED = "kickside.channel.responder.events:session.completed"
local LAZY_ASSESS_SYSTEM = [[You decide whether a channel responder should answer an accumulated batch of channel messages.
Return exactly RESPOND or SKIP.
Choose SKIP for greetings, acknowledgements, unrelated chatter, or messages outside the responder policy.
Choose RESPOND only when the responder should generate a public reply.]]

-- One conversation's session trigger. The hub spawns one bridge per route.key UNDER
-- THE RUN-AS FRAME (a linked user for a DM, the installer for a channel), so the
-- bridge owns a wippy.session that runs as that identity. It is identity-agnostic:
-- it derives actor + scope from its own frame, never resolving who to run as. On
-- startup it ensures the audit thread and the session; each inbound turn is
-- appended to the thread, its attachments pulled as the run-as user, and the text
-- forwarded to the session. Streamed CONTENT is buffered and one reply per turn is
-- flushed (on the session's IDLE update) through the connection's reply provider --
-- which reads the connection token at the system level, so no connection access is
-- required. It idle-terminates after consts.SESSION_TTL_MS; the session rows
-- persist, so a later bridge with the same session id rehydrates, or a rotated id
-- starts fresh.
local M = {}

type Map = { [string]: any }
type ReplyConfig = {
    component_id: string?,
    resource_id: string?,
}
type StatusHandle = {
    component_id: string?,
    resource_id: string?,
    message_ref: string?,
    sent: boolean?,
    editable: boolean?,
}

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function display_name(inbound: any): string
    local name = trim(inbound.external_display_name)
    if name ~= "" then return name end
    name = trim(inbound.external_username)
    if name ~= "" then return name end
    return trim(inbound.external_user_id)
end

-- The actor the run-as frame owns: a DM subject's id, or the channel installer.
-- Sourced from the bridge's own actor so the session and uploads are owned by
-- that identity, never threaded over the wire.
local function actor_id(actor: any): string
    return trim(actor:id())
end

-- The session applies config.active_traits as the agent's FULL trait set (a replace,
-- not an append). To add the responder's extra traits ON TOP of the agent's own,
-- resolve the agent's own traits and concatenate, deduped by id. Returns nil when
-- there are no extras, so the agent keeps its own traits untouched. Best-effort: if
-- the agent's own traits can't be resolved, the extras still apply.
local function resolve_active_traits(start: any, actor: any, scope: any): { any }?
    local extra = type((start :: any).traits) == "table" and (start :: any).traits or {}
    if #(extra :: { any }) == 0 then return nil end
    local trait_contexts = type((start :: any).trait_contexts) == "table" and (start :: any).trait_contexts or {}

    local own: { any } = {}
    local def, def_err = contract.get(AGENT_RESOLVER)
    if def_err then
        logger:warn("agent resolver contract unavailable", { error = tostring(def_err) })
    elseif def then
        local inst, open_err = (def :: any):with_actor(actor):with_scope(scope):open()
        if open_err then
            logger:warn("agent resolver open failed", { error = tostring(open_err) })
        elseif inst then
            local spec, resolve_err = inst:resolve({ agent_id = trim((start :: any).agent_id) })
            if resolve_err then
                logger:warn("agent traits resolve failed", { error = tostring(resolve_err) })
            elseif type(spec) == "table" and type((spec :: any).traits) == "table" then
                own = (spec :: any).traits :: { any }
            end
        end
    end

    local seen: { [string]: boolean } = {}
    local out: { any } = {}
    for _, t in ipairs(own) do
        local id = type(t) == "table" and trim((t :: any).id) or trim(t)
        if id ~= "" and not seen[id] then seen[id] = true; out[#out + 1] = t end
    end
    for _, raw in ipairs(extra :: { any }) do
        local id = trim(raw)
        if id ~= "" and not seen[id] then
            seen[id] = true
            local tctx = (trait_contexts :: Map)[id]
            out[#out + 1] = type(tctx) == "table" and { id = id, context = tctx } or { id = id }
        end
    end
    return out
end

-- Build the session config the session process reads on every turn. agent_id
-- lives in config, so redirects can switch the agent in place. active_traits
-- (when set) is the full trait set the session applies on top of the agent.
local function session_config(thread_id: string, start: any, active_traits: { any }?): Map
    local base = session_consts.get_config()
    local config: Map = {
        token_checkpoint_threshold = base.token_checkpoint_threshold,
        max_message_limit = base.max_message_limit,
        checkpoint_function_id = base.checkpoint_function_id,
        title_function_id = base.title_function_id,
        delegation_func_id = base.delegation_func_id,
        enable_agent_cache = base.enable_agent_cache,
        delegation_description_suffix = base.delegation_description_suffix,
        agent_id = trim((start :: any).agent_id),
        thread_id = thread_id,
    }
    if type(active_traits) == "table" and #(active_traits :: { any }) > 0 then
        config.active_traits = active_traits
    end
    return config
end

local function ensure_session(session_id: string, thread_id: string, start: any, user_id: string, active_traits: { any }?): (boolean?, string?)
    local session_kind = trim((start :: any).session_kind) ~= "" and trim((start :: any).session_kind) or "CHAT"
    local result, ensure_err = session_service.ensure({
        session_id = session_id,
        user_id = user_id,
        kind = session_kind,
        meta = {},
        config = session_config(thread_id, start, active_traits),
        primary_context_data = {},
    })
    if ensure_err then
        return nil, tostring(ensure_err)
    end
    if not result then
        return nil, "session service returned no result"
    end
    return result.created == true, nil
end

-- Spawn the session process under the bridge's own frame (the run-as identity the
-- hub spawned this bridge with). The session derives its identity from this frame
-- and runs the agent as that subject/installer.
local function spawn_session_process(actor: any, scope: any, session_id: string, thread_id: string, user_id: string, create_new: boolean): (any?, string?)
    local pid, err = process.with_context({ session_id = session_id, user_id = user_id })
        :with_actor(actor)
        :with_scope(scope)
        :spawn_linked_monitored(session_consts.PROCESS.SESSION_ID :: string, consts.HUB_HOST, {
            session_id = session_id,
            thread_id = thread_id,
            user_id = user_id,
            parent_pid = process.pid(),
            create = create_new,
        })
    if err then return nil, "spawn session: " .. tostring(err) end
    return pid, nil
end

-- Forward a turn to the session. Provider attachments are retained for trace,
-- while materialized upload IDs are what the session stores and tools read.
local function send_session_message(session_pid: string, text: string, attachments: { any }?, file_uuids: { string }?)
    local data: Map = { text = text }
    if type(attachments) == "table" and #(attachments :: { any }) > 0 then
        data.attachments = attachments
    end
    if type(file_uuids) == "table" and #(file_uuids :: { string }) > 0 then
        data.file_uuids = file_uuids
    end
    process.send(session_pid, session_consts.TOPICS.MESSAGE :: string, { data = data })
end

local function copy_table(value: any): Map
    local out: Map = {}
    if type(value) ~= "table" then return out end
    for k, v in pairs(value :: Map) do out[k] = v end
    return out
end

local function bounded_limit(value: any): number
    local n = tonumber(value) or 10
    if n < 1 then return 1 end
    if n > 50 then return 50 end
    return math.floor(n)
end

local function activation_context(start: any): Map
    local cfg = type((start :: any).activation_context) == "table" and (start :: any).activation_context or {}
    return cfg :: Map
end

local function include_recent_context(start: any): boolean
    return (activation_context(start).enabled == true)
end

local function recent_context_prompt(start: any, reply_config: any, first_inbound: any): string
    if not include_recent_context(start) then return "" end
    local component_id = trim(reply_config.component_id)
    local resource_id = trim(reply_config.resource_id)
    if component_id == "" or resource_id == "" then return "" end

    local svc, open_err = component.open_private(component_id, CHANNEL_CONTEXT_CONTRACT)
    if open_err or not svc then
        logger:warn("channel context unavailable", {
            component_id = component_id,
            resource_id = resource_id,
            error = tostring(open_err),
        })
        return ""
    end

    local result, read_err = (svc :: any):read_recent({
        resource_id = resource_id,
        before_external_message_id = trim((first_inbound :: any).external_message_id),
        limit = bounded_limit(activation_context(start).limit),
    })
    if read_err or type(result) ~= "table" or (result :: Map).success ~= true then
        logger:warn("channel context read failed", {
            component_id = component_id,
            resource_id = resource_id,
            error = tostring(read_err or (type(result) == "table" and (result :: Map).error or "unknown error")),
        })
        return ""
    end

    local messages = type((result :: Map).messages) == "table" and (result :: Map).messages or {}
    local lines: { string } = {}
    for i = #(messages :: { any }), 1, -1 do
        local msg = (messages :: { any })[i]
        if type(msg) == "table" then
            local text = trim((msg :: Map).text)
            if text ~= "" then
                local who = trim((msg :: Map).external_display_name)
                if who == "" then who = trim((msg :: Map).external_username) end
                if who == "" then who = trim((msg :: Map).external_user_id) end
                lines[#lines + 1] = (who ~= "" and (who .. ": " .. text) or text)
            end
        end
    end
    if #lines == 0 then return "" end
    return "Recent channel context before this activation:\n" .. table.concat(lines, "\n")
end

local function ingest_attachments(inbound: any, actor_id: string, thread_id: string): ({ string }, { any })
    local file_uuids: { string } = {}
    local attachments: { any } = {}
    local raw_attachments = type((inbound :: any).attachments) == "table" and (inbound :: any).attachments or {}

    for _, raw in ipairs(raw_attachments :: { any }) do
        local attachment = copy_table(raw)
        attachment.actor_id = actor_id
        attachment.source = trim((inbound :: any).provider)
        attachment.thread_id = thread_id
        attachment.external_message_id = trim((inbound :: any).external_message_id)

        local upload, upload_err = attachment_ingest.ingest(attachment)
        if upload and type(upload.uuid) == "string" and upload.uuid ~= "" then
            attachment.upload_id = upload.uuid
            file_uuids[#file_uuids + 1] = upload.uuid
        else
            attachment.ingest_error = tostring(upload_err or "unknown error")
            logger:warn("attachment ingest failed", {
                key = trim((inbound :: any).channel_id),
                filename = trim(attachment.filename),
                error = attachment.ingest_error,
            })
        end

        attachments[#attachments + 1] = attachment
    end

    return file_uuids, attachments
end

local function appended_event(result: any): any?
    if type(result) ~= "table" then return nil end
    if type((result :: Map).event) == "table" then return (result :: Map).event end
    if type((result :: Map).id) == "string" then return result end
    return nil
end

local function prepare_session_turn(params: any): any
    params = type(params) == "table" and params or {}
    local inbound = type((params :: Map).inbound) == "table" and (params :: Map).inbound or nil
    if type(inbound) ~= "table" then return nil end

    local user_id = trim((params :: Map).user_id)
    local thread_id = trim((params :: Map).thread_id)
    local key = trim((params :: Map).key)
    local thread_class = trim((params :: Map).thread_class) ~= "" and trim((params :: Map).thread_class) or consts.THREAD_CLASS
    local inbound_event = trim((params :: Map).inbound_event) ~= "" and trim((params :: Map).inbound_event) or consts.EVENT.INBOUND
    local file_uuids, attachments = ingest_attachments(inbound, user_id, thread_id)
    local thread_inbound = copy_table(inbound)
    thread_inbound.attachments = attachments
    if #file_uuids > 0 then thread_inbound.file_uuids = file_uuids end

    local appended, append_err = thread_bridge.append_inbound(thread_id, thread_inbound, {
        event_type = inbound_event,
        external_source = thread_class .. "." .. trim((inbound :: any).provider),
        trace_context = (thread_inbound :: Map).trace_context,
    })
    if append_err then
        local err = "append inbound failed for " .. key .. ": " .. tostring(append_err)
        logger:error("append inbound failed", { key = key, error = tostring(append_err) })
        return nil, err
    end
    if type(appended) == "table" and (appended :: Map).duplicate == true then
        return nil
    end
    local source_event = appended_event(appended)
    local turn_trace = source_event and trace_context.common({ source_event }) or nil
    local text = type((inbound :: any).text) == "string" and (inbound :: any).text or ""
    local author = display_name(inbound)
    local body = author ~= "" and text ~= "" and (author .. ": " .. text) or text
    if body == "" and #attachments == 0 and #file_uuids == 0 then return nil end
    return {
        body = body,
        attachments = attachments,
        file_uuids = file_uuids,
        trace_context = turn_trace,
        id = source_event and (source_event :: Map).id or nil,
    }
end

local function main(args: any)
    args = type(args) == "table" and args or {}
    local route: Map = (type(args.route) == "table" and args.route or {}) :: Map
    local key = trim(route.key)
    if key == "" then error("route.key is required") end
    local session_id = trim(route.session_id)
    if session_id == "" then error("route.session_id is required") end

    local thread = type(route.thread) == "table" and route.thread or {}
    local start = type(route.start) == "table" and route.start or {}
    local reply = type(route.reply) == "table" and route.reply or {}
    local first_inbound = type(route.inbound) == "table" and route.inbound or {}

    local thread_class = trim((thread :: any).class) ~= "" and trim((thread :: any).class) or consts.THREAD_CLASS
    local inbound_event = trim((thread :: any).inbound_event) ~= "" and trim((thread :: any).inbound_event) or consts.EVENT.INBOUND
    local reply_sent_event = trim((reply :: any).sent_event) ~= "" and trim((reply :: any).sent_event) or consts.EVENT.REPLY_SENT
    local reply_failed_event = trim((reply :: any).failed_event) ~= "" and trim((reply :: any).failed_event) or consts.EVENT.REPLY_FAILED
    local responder_component_id = trim((start :: any).component_id)
    local reply_config: ReplyConfig = {
        component_id = trim((reply :: any).component_id),
        resource_id = trim((reply :: any).resource_id),
    }
    local function new_status_handle(): StatusHandle
        return {
            component_id = reply_config.component_id,
            resource_id = reply_config.resource_id,
        }
    end
    local tool_status_handle: StatusHandle = new_status_handle()
    local ttl_ms = consts.SESSION_TTL_MS

    local registry_name = consts.BRIDGE_REGISTRY_PREFIX .. key
    process.set_options({ trap_links = true, upgradable = false })
    process.registry.register(registry_name, process.pid())

    -- The hub handed this bridge the first inbound turn and marked it seen (witnessed)
    -- on spawn. A startup abort leaves that turn unrecorded, so signal the hub to
    -- release its dedup entry for redelivery before exiting: an infrastructure
    -- failure during startup must surface as an error and stay retriable, never a
    -- silent return that drops the turn.
    local function release_first_turn()
        process.send(consts.HUB_NAME, consts.HUB_TOPIC, {
            kind = "startup_failed",
            key = key,
            message_id = trim((first_inbound :: any).external_message_id),
        })
        process.registry.unregister(registry_name)
    end

    local function release_failed_turn(inbound: any)
        process.send(consts.HUB_NAME, consts.HUB_TOPIC, {
            kind = "append_failed",
            key = key,
            message_id = trim(type(inbound) == "table" and (inbound :: any).external_message_id or nil),
        })
        process.registry.unregister(registry_name)
    end

    -- The bridge's own frame is the run-as identity the hub spawned it with. It
    -- backs the thread ops and the session spawn.
    local self_actor = security.actor()
    local self_scope = security.scope()
    if not self_actor or not self_scope then
        logger:error("bridge has no run-as identity", { key = key })
        release_first_turn()
        error("bridge has no run-as identity for " .. key)
    end
    local user_id = actor_id(self_actor)
    if user_id == "" then
        logger:error("bridge run-as identity has no actor", { key = key })
        release_first_turn()
        error("bridge run-as identity has no actor for " .. key)
    end

    -- Ensure the audit thread for this conversation (under the run-as frame).
    local thread_id, _, terr = thread_bridge.ensure_thread(first_inbound, {
        thread_class = thread_class,
        title_prefix = trim((thread :: any).title_prefix),
        managed_by = trim((thread :: any).managed_by),
        receive_target = trim((thread :: any).receive_target),
        role = trim((thread :: any).role),
        user_ids = type((thread :: any).user_ids) == "table" and (thread :: any).user_ids or { user_id },
    })
    if terr or not thread_id then
        logger:error("ensure thread failed", { key = key, error = tostring(terr) })
        release_first_turn()
        error("ensure thread failed for " .. key .. ": " .. tostring(terr))
    end

    local active_traits = resolve_active_traits(start, self_actor, self_scope)
    local session_pid: string? = nil
    local session_created: boolean? = nil
    local first_session_turn = true
    local session_lifecycle_active = false
    logger:info("bridge started", { key = key, thread_id = thread_id, session_id = session_id, user_id = user_id })

    local response_buffer = ""
    local current_reply_trace = nil
    local idle_timer = time.after(ttl_ms .. "ms")
    local turn_timer = nil
    local inbox = process.inbox()
    local events_ch = process.events()
    local lazy_mode = reply_policy.lazy_enabled(start)
    local wait_ms = reply_policy.lazy_wait_ms(start)

    local function now_rfc3339(): string
        return time.now():utc():format(time.RFC3339)
    end

    local function emit_session_event(etype: string, reason: string?): (boolean, string?)
        if responder_component_id == "" then return true, nil end
        local body: Map = {
            session_id = session_id,
            thread_id = thread_id,
            route_key = key,
            at = now_rfc3339(),
        }
        if reason ~= nil and reason ~= "" then body.reason = reason end
        local last_err: string? = nil
        for attempt = 1, 3 do
            local _, emit_err = thread_bridge.emit_responder_event(responder_component_id, etype, body)
            if not emit_err then return true, nil end
            last_err = tostring(emit_err)
            if attempt < 3 then time.sleep("100ms") end
        end
        logger:error("responder session event emit failed", {
            key = key,
            component_id = responder_component_id,
            event_type = etype,
            error = last_err,
        })
        return false, last_err or "responder session event emit failed"
    end

    local function mark_session_activated(): (boolean, string?)
        if session_lifecycle_active then return true, nil end
        local ok, err = emit_session_event(SESSION_ACTIVATED, nil)
        if not ok then return false, err end
        session_lifecycle_active = true
        return true, nil
    end

    local function mark_session_completed(reason: string): (boolean, string?)
        if not session_lifecycle_active then return true, nil end
        local ok, err = emit_session_event(SESSION_COMPLETED, reason)
        if not ok then return false, err end
        session_lifecycle_active = false
        return true, nil
    end

    local function assess_lazy_turn(body: string): boolean
        if not reply_policy.should_assess(start, session_pid ~= nil) then return true end
        local p = prompt.new()
        p:add_system(LAZY_ASSESS_SYSTEM)
        p:add_user("Responder policy:\n" .. (reply_policy.config(start).prompt or "") .. "\n\nMessages:\n" .. body)
        local resp, err = llm.generate(p, reply_policy.assessor_options(start))
        if err or type(resp) ~= "table" then
            logger:warn("lazy response assessment failed; forwarding to agent", { key = key, error = tostring(err) })
            return true
        end
        local verdict = trim((resp :: Map).result):upper()
        return verdict:find("RESPOND", 1, true) ~= nil
    end

    local function flush_response()
        local reply_trace = current_reply_trace
        current_reply_trace = nil
        if response_buffer == "" then return end
        local content = response_buffer
        response_buffer = ""
        if not reply_policy.should_send_reply(start, content) then
            return
        end
        local result = nil
        local err = nil
        local updated = false
        if tool_status_handle.sent == true then
            result, err, updated = reply_sender.finish_status(tool_status_handle, content)
            tool_status_handle = new_status_handle()
        else
            result, err = reply_sender.send_reply(reply_config, content)
        end
        if err then
            logger:error("reply failed", { key = key, error = err })
            thread_bridge.append_reply_event(thread_id, reply_failed_event, {
                text = content,
                status = "send_failed",
                error = tostring(err),
                component_id = reply_config.component_id,
                resource_id = reply_config.resource_id,
            }, reply_trace)
            return
        end
        thread_bridge.append_reply_event(thread_id, reply_sent_event, {
            text = content,
            status = updated and "updated_status" or "sent",
            message_ref = type(result) == "table" and (result :: Map).message_ref or nil,
            component_id = reply_config.component_id,
            resource_id = reply_config.resource_id,
        }, reply_trace)
    end

    local function send_tool_status(payload: any)
        local t = type(payload) == "table" and trim((payload :: Map).type) or ""
        local text = reply_policy.tool_status_text(t, type(payload) == "table" and (payload :: Map).function_name or nil)
        if text == nil or text == "" then return end
        local reply_trace = current_reply_trace
        local handle, err, changed = reply_sender.update_status(tool_status_handle, text)
        tool_status_handle = handle
        if changed ~= true and not err then return end
        if err then
            logger:error("tool status reply failed", { key = key, error = err })
            thread_bridge.append_reply_event(thread_id, reply_failed_event, {
                text = text,
                status = "tool_status_send_failed",
                error = tostring(err),
                component_id = reply_config.component_id,
                resource_id = reply_config.resource_id,
            }, reply_trace)
            return
        end
        thread_bridge.append_reply_event(thread_id, reply_sent_event, {
            text = text,
            status = "tool_status",
            message_ref = tool_status_handle.message_ref,
            component_id = reply_config.component_id,
            resource_id = reply_config.resource_id,
        }, reply_trace)
    end

    -- Turn batching. The session runs one agent step per USER message it receives,
    -- so a fast typer's burst would be one query + one reply each. Immediate mode
    -- keeps the existing behavior: first message goes through now, messages that
    -- arrive while the agent is running release as one follow-up. Lazy mode waits
    -- for a quiet window before each agent turn, so multiple channel messages become
    -- one session message before the agent decides whether to answer.
    local session_busy = false
    local pending: { any } = {}

    local function schedule_lazy_flush()
        turn_timer = time.after(tostring(wait_ms) .. "ms")
    end

    -- prepare_turn: materialize provider attachment refs into upload IDs and append
    -- the audited inbound (every message is logged), returning the session-ready
    -- parts -- or nil for an empty turn.
    local function prepare_turn(inbound: any): any
        return prepare_session_turn({
            inbound = inbound,
            user_id = user_id,
            thread_id = thread_id,
            thread_class = thread_class,
            inbound_event = inbound_event,
            key = key,
        })
    end

    local system_prompt = type((start :: any).system_prompt) == "string" and (start :: any).system_prompt or ""
    local start_context = trim((start :: any).context)
    if start_context ~= "" then
        system_prompt = system_prompt ~= "" and (system_prompt .. "\n\nContext:\n" .. start_context) or ("Context:\n" .. start_context)
    end
    local channel_context = recent_context_prompt(start, reply_config, first_inbound)
    if channel_context ~= "" then
        system_prompt = system_prompt ~= "" and (system_prompt .. "\n\n" .. channel_context) or channel_context
    end

    local function ensure_session_runtime(): (string?, string?)
        if session_pid ~= nil then return session_pid, nil end
        local created, create_err = ensure_session(session_id, thread_id, start, user_id, active_traits)
        if created == nil then return nil, "ensure session: " .. tostring(create_err) end
        local session_pid_raw, spawn_err = spawn_session_process(self_actor, self_scope, session_id, thread_id, user_id, created == true)
        if spawn_err or not session_pid_raw then return nil, "spawn session: " .. tostring(spawn_err) end
        session_created = created == true
        session_pid = session_pid_raw :: string
        logger:info("session attached", { key = key, thread_id = thread_id, session_id = session_id, created = session_created == true })
        local active_ok, active_err = mark_session_activated()
        if not active_ok then
            process.cancel(session_pid_raw :: string, "5s")
            session_pid = nil
            return nil, "mark session active: " .. tostring(active_err)
        end
        return session_pid, nil
    end

    local function send_turn(body: string, attachments: { any }, file_uuids: { string }, turn_trace: any?)
        local pid, session_err = ensure_session_runtime()
        if not pid then
            logger:error("session attach failed", { key = key, error = session_err })
            process.registry.unregister(registry_name)
            return
        end
        if first_session_turn then
            first_session_turn = false
            if session_created == true and system_prompt ~= "" then
                body = "Channel responder instructions:\n" .. system_prompt .. "\n\nIncoming channel message:\n" .. body
            end
        end
        session_busy = true
        current_reply_trace = turn_trace
        send_session_message(pid, body, attachments, file_uuids)
    end

    -- Release everything that piled up during the agent's run as a single turn.
    local function flush_pending()
        if #pending == 0 then session_busy = false; return end
        local batch = pending
        pending = {}
        local bodies: { string } = {}
        local atts: { any } = {}
        local uuids: { string } = {}
        for _, p in ipairs(batch) do
            if p.body ~= "" then bodies[#bodies + 1] = p.body end
            for _, a in ipairs(p.attachments :: { any }) do atts[#atts + 1] = a end
            for _, u in ipairs(p.file_uuids :: { string }) do uuids[#uuids + 1] = u end
        end
        local body = table.concat(bodies, "\n")
        local assess_now = reply_policy.should_assess(start, session_pid ~= nil)
        if assess_now and not assess_lazy_turn(body) then
            session_busy = false
            return
        end
        send_turn(body, atts, uuids, trace_context.common(batch))
    end

    local function handle_turn(inbound: any)
        local p, prep_err = prepare_turn(inbound)
        if prep_err then
            release_failed_turn(inbound)
            error(tostring(prep_err))
        end
        if not p then return end
        if reply_policy.should_assess(start, session_pid ~= nil) then
            pending[#pending + 1] = p
            if not session_busy then schedule_lazy_flush() end
        elseif session_busy then
            pending[#pending + 1] = p
        else
            send_turn(p.body :: string, p.attachments :: { any }, p.file_uuids :: { string }, p.trace_context)
        end
    end

    handle_turn(first_inbound)

    while true do
        local cases = {
            inbox:case_receive(),
            events_ch:case_receive(),
            idle_timer:case_receive(),
        }
        if turn_timer ~= nil then cases[#cases + 1] = (turn_timer :: any):case_receive() end
        local result = channel.select(cases)

        if result.channel == idle_timer then
            logger:info("session idle timeout", { key = key, session_id = session_id })
            flush_response()
            mark_session_completed("idle_timeout")
            if session_pid ~= nil then process.cancel(session_pid :: string, "5s") end
            break
        elseif turn_timer ~= nil and result.channel == turn_timer then
            turn_timer = nil
            if not session_busy then flush_pending() end
        elseif result.channel == events_ch then
            local event = result.value
            if event.kind == process.event.CANCEL then
                flush_response()
                mark_session_completed("cancelled")
                if session_pid ~= nil then process.cancel(session_pid :: string, "5s") end
                break
            end
            if (event.kind == process.event.EXIT or event.kind == process.event.LINK_DOWN) and event.from == session_pid then
                logger:warn("session process exited", { key = key, kind = event.kind })
                flush_response()
                mark_session_completed("session_exit")
                break
            end
        elseif result.channel == inbox then
            local msg = result.value
            local topic = msg:topic()
            local payload = msg:payload():data()

            if topic == "messages" then
                idle_timer = time.after(ttl_ms .. "ms")
                handle_turn(type(payload.inbound) == "table" and payload.inbound or nil)
            else
                local prefix = session_consts.TOPIC_PREFIXES.SESSION .. session_id
                if string.sub(topic, 1, #prefix) == prefix then
                    local t = payload.type
                    if t == session_consts.UPSTREAM_TYPES.CONTENT then
                        local content = type(payload.content) == "string" and payload.content or ""
                        if content ~= "" then response_buffer = response_buffer .. content end
                    elseif t == session_consts.UPSTREAM_TYPES.UPDATE then
                        -- IDLE = the agent's turn (its whole op queue) drained: send
                        -- the reply, then release anything that arrived mid-run as one
                        -- combined follow-up turn.
                        if payload.status == session_consts.STATUS.IDLE then
                            flush_response()
                            idle_timer = time.after(ttl_ms .. "ms")
                            if reply_policy.should_assess(start, session_pid ~= nil) then
                                session_busy = false
                                if #pending > 0 then schedule_lazy_flush() end
                            else
                                flush_pending()
                            end
                        end
                    elseif t == session_consts.UPSTREAM_TYPES.ERROR then
                        -- The errored step still drains the queue -> an IDLE follows,
                        -- which releases any pending batch; nothing to do here.
                        logger:error("session error", { key = key, code = payload.code, message = payload.message })
                    elseif t == session_consts.UPSTREAM_TYPES.FUNCTION_CALL
                        or t == session_consts.UPSTREAM_TYPES.FUNCTION_ERROR then
                        send_tool_status(payload)
                    end
                end
            end
        end
    end

    mark_session_completed("bridge_stopped")
    process.registry.unregister(registry_name)
    logger:info("bridge stopped", { key = key, session_id = session_id })
end

M.main = main
M._prepare_session_turn_for_test = prepare_session_turn
return M

