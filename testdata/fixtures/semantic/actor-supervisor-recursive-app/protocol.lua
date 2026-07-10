type AppError = {
    code: string,
    message: string,
}

type TaskMessage = {
    kind: "task",
    id: string,
    route_id: string,
    payload: {[string]: string},
}

type TimerMessage = {
    kind: "timer",
    id: string,
    due_at: number,
}

type Envelope = TaskMessage | TimerMessage

type EnvelopeResult = {ok: true, value: Envelope} | {ok: false, error: AppError}
type StringResult = {ok: true, value: string} | {ok: false, error: AppError}

type Route = {
    id: string,
    label: string,
    next: Route?,
}

type ActorState = {
    processed: {[string]: Envelope},
    counters: {[string]: number},
    last_id: string?,
}

type Actor = {
    id: string,
    routes: {[string]: Route},
    handlers: {[string]: (Actor, Envelope) -> StringResult},
    state: ActorState,
    add_route: (self: Actor, route: Route) -> Actor,
    register: (self: Actor, kind: string, handler: (Actor, Envelope) -> StringResult) -> Actor,
    dispatch: (self: Actor, message: Envelope) -> StringResult,
}

local M = {}
M.AppError = AppError
M.TaskMessage = TaskMessage
M.TimerMessage = TimerMessage
M.Envelope = Envelope
M.EnvelopeResult = EnvelopeResult
M.StringResult = StringResult
M.Route = Route
M.ActorState = ActorState
M.Actor = Actor

function M.err(code: string, message: string): AppError
    return {code = code, message = message}
end

return M
