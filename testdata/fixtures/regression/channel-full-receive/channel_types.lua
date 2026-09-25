-- The wire vocabulary between a surface and the shared runtime. A transport
-- produces an Envelope; a surface (DM or channel) turns it into a Route -- the one
-- struct the runtime consumes. Keeping these named here makes the seam traceable:
-- a surface fills a Route, the hub routes by route.key, the bridge reads the rest.

local M = {}

-- Normalized transport message. Discord/Slack/etc produce only this.
type Envelope = {
    provider: string,
    reply_component_id: string,
    reply_resource_id: string?,
    channel_id: string,
    external_user_id: string,
    external_username: string?,
    external_display_name: string?,
    external_message_id: string?,
    text: string,
    attachments: { any }?,
    trace_context: any?,
}

-- Who the session runs as. subject -> a linked Kickside user's live scope (DM);
-- frozen -> a captured execution identity (channel responder; the installer).
type RunAs = {
    mode: string,
    subject_id: string?,
    identity: any?,
}

-- The audit thread's shape. The surface chooses it; the runtime ensures + appends.
-- receive_target is the surface's own inbound contract id (e.g. kickside.dm:receive);
-- it is stamped onto the thread so a re-dispatch (inject) resolves the receive seam
-- from the thread itself instead of the engine branching on the surface.
type ThreadShape = {
    class: string,
    title_prefix: string,
    managed_by: string,
    receive_target: string,
    role: string,
    user_ids: { string },
    inbound_event: string,
}

-- Session start config. agent_id is not an abstraction -- it is start config the
-- session reads each turn (a redirect rewrites it in place). traits/trait_contexts
-- are extra agent traits (added on top of the agent's own); context is free-text
-- channel context seeded into the first real turn.
type Start = {
    agent_id: string,
    session_kind: string,
    system_prompt: string?,
    traits: { string }?,
    trait_contexts: any?,
    context: string?,
    session_policy: any?,
}

-- Where the reply goes + the thread events the runtime appends for it.
type Reply = {
    component_id: string,
    resource_id: string,
    sent_event: string,
    failed_event: string,
}

-- The one abstraction the runtime needs. A surface builds it; the hub keys live
-- bridges by route.key; the bridge reads thread/start/reply/inbound. Attachments
-- ride on inbound as provider-neutral refs; the bridge materializes them into
-- upload IDs before the session turn is stored.
type Route = {
    key: string,
    session_id: string,
    run_as: RunAs,
    thread: ThreadShape,
    start: Start,
    reply: Reply,
    inbound: Envelope,
}

-- What a surface hands the shared route_turn orchestrator: the route key, the
-- session (provider, subject) to map, the identity + thread + start config, the
-- reply event pair, and the trace domain + extra correlation parts. The
-- orchestrator resolves the session id, derives the trace, fills the reply target
-- off inbound, and assembles the Route -- the surface never touches those.
type RouteSpec = {
    route_key: string,
    session_provider: string,
    session_subject: string,
    run_as: RunAs,
    thread: ThreadShape,
    start: Start,
    reply_sent_event: string,
    reply_failed_event: string,
    trace_domain: string,
    trace_parts: { string },
    session_policy: any?,
}

M.Envelope = Envelope
M.RunAs = RunAs
M.ThreadShape = ThreadShape
M.Start = Start
M.Reply = Reply
M.Route = Route
M.RouteSpec = RouteSpec

return M

