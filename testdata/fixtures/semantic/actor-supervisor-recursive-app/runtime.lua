local protocol = require("protocol")

local M = {}

local function bump(state: protocol.ActorState, key: string)
    local current = state.counters[key]
    if current then
        state.counters[key] = current + 1
    else
        state.counters[key] = 1
    end
end

function M.route(id: string, label: string, next: protocol.Route?): protocol.Route
    return {id = id, label = label, next = next}
end

function M.new_actor(id: string): protocol.Actor
    local actor: protocol.Actor = {
        id = id,
        routes = {},
        handlers = {},
        state = {
            processed = {},
            counters = {},
            last_id = nil,
        },
        add_route = function(self: protocol.Actor, route: protocol.Route): protocol.Actor
            self.routes[route.id] = route
            return self
        end,
        register = function(self: protocol.Actor, kind: string, handler: (protocol.Actor, protocol.Envelope) -> protocol.StringResult): protocol.Actor
            self.handlers[kind] = handler
            return self
        end,
        dispatch = function(self: protocol.Actor, message: protocol.Envelope): protocol.StringResult
            local handler = self.handlers[message.kind]
            if not handler then
                return {ok = false, error = protocol.err("missing_handler", message.kind)}
            end
            self.state.processed[message.id] = message
            self.state.last_id = message.id
            bump(self.state, message.kind)
            return handler(self, message)
        end,
    }
    return actor
end

function M.task_handler(actor: protocol.Actor, message: protocol.Envelope): protocol.StringResult
    if message.kind ~= "task" then
        return {ok = false, error = protocol.err("wrong_kind", message.kind)}
    end
    local route = actor.routes[message.route_id]
    if not route then
        return {ok = false, error = protocol.err("missing_route", message.route_id)}
    end
    local current = route
    local last_label = current.label
    while current.next do
        current = current.next
        last_label = current.label
    end
    local owner = message.payload.owner
    if owner then
        return {ok = true, value = message.id .. ":" .. last_label .. ":" .. owner}
    end
    return {ok = true, value = message.id .. ":" .. last_label}
end

function M.timer_handler(_actor: protocol.Actor, message: protocol.Envelope): protocol.StringResult
    if message.kind ~= "timer" then
        return {ok = false, error = protocol.err("wrong_kind", message.kind)}
    end
    return {ok = true, value = message.id .. ":" .. tostring(message.due_at)}
end

return M
