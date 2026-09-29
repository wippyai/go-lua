local M = {}

function M.reset()
    M.state = {
        sent = {},
        private_patches = {},
        private_reads = {},
        private_state = {},
        events = {},
        status_reads = {},
        send_error = nil,
        -- components: id -> { component_id, impl_id?, meta } backing the indexed
        -- component.list_system reads (routing lookup, duplicate guard, provider_of,
        -- agent existence). responders: id -> { state, execution_identity } backing
        -- the single-component runtime_reader.get_runtime read.
        components = {},
        responders = {},
        set_meta_calls = {},
        -- inject: the thread kickside.core:threads:find returns, the receive
        -- contracts registered by id, and the inbound each captured on receive.
        thread = nil,
        thread_find_err = nil,
        receive_targets = {},
        receive_calls = {},
        -- witness: the inbound-witness thread writes the ingress performs for a
        -- responder-covered channel whose turn does not route (paused / agent gone).
        witness = {},
        -- witness_run_as_err: makes run_as.resolve fail (installer principal
        -- unresolvable); witness_ensure_err: makes the witness ensure_thread fail.
        -- Both drive the witness path's truthful skip.
        witness_run_as_err = nil,
        witness_ensure_err = nil,
    }
end

-- add_thread registers the row kickside.core:threads:find hands back. attrs is the
-- hydrated table a single-thread read returns (never a JSON string).
function M.add_thread(attrs)
    M.state.thread = { id = "thread-1", component_id = "thread-1", attrs = type(attrs) == "table" and attrs or {} }
end

-- add_receive_target registers a surface receive contract by id; inject resolves it
-- from the thread's attrs.receive_target and calls :receive(inbound), captured here.
function M.add_receive_target(id, result)
    M.state.receive_targets[id] = result or { accepted = true, status = "routed" }
end

-- add_responder registers an installed channel responder in both surfaces: its
-- routing keys in component meta (indexed lookup) and its private state + frozen
-- execution identity (single-component read).
function M.add_responder(id, opts)
    opts = type(opts) == "table" and opts or {}
    local connection_id = opts.connection_id or "conn-1"
    local channel_id = opts.channel_id or "chan-1"
    M.state.components[id] = {
        component_id = id,
        impl_id = opts.impl_id or "kickside.channel.responder:channel_responder",
        meta = {
            connection_id = connection_id,
            channel_id = channel_id,
            title = opts.title or ("Responder " .. id),
            status = opts.paused and "paused" or "active",
        },
    }
    M.state.responders[id] = {
        state = {
            connection_id = connection_id,
            channel_id = channel_id,
            channel_name = opts.channel_name or "",
            agent_id = opts.agent_id or "agent-1",
            agent_name = opts.agent_name or "",
            title = opts.title or ("Responder " .. id),
            paused = opts.paused == true,
            traits = opts.traits or {},
            trait_contexts = opts.trait_contexts or {},
            context = opts.context or "",
            response_policy = opts.response_policy or {},
            session_policy = opts.session_policy or {},
            system_prompt = opts.system_prompt,
        },
        execution_identity = opts.execution_identity or {
            actor_id = "installer-1",
            actor_context = { version = 1, scope_id = "app:member", claims = {} },
        },
    }
end

-- add_component registers a plain component (connection, agent) so existence and
-- provider reads resolve it.
function M.add_component(id, meta)
    M.state.components[id] = { component_id = id, meta = type(meta) == "table" and meta or {} }
end

M.reset()

return M

