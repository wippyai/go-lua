package checktest

import "testing"

func TestImportedRecursiveResultReturnKeepsLoopGuardedFieldPresent(t *testing.T) {
	protocol := CheckAndExport(`
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
type StringResult = {ok: true, value: string} | {ok: false, error: AppError}

type Route = {
    id: string,
    label: string,
    next: Route?,
}

type Actor = {
    id: string,
    routes: {[string]: Route},
}

local M = {}
M.AppError = AppError
M.TaskMessage = TaskMessage
M.TimerMessage = TimerMessage
M.Envelope = Envelope
M.StringResult = StringResult
M.Route = Route
M.Actor = Actor

function M.err(code: string, message: string): AppError
    return {code = code, message = message}
end

return M
`, "protocol", WithStdlib())
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v", protocol.Errors)
	}

	result := Check(`
local protocol = require("protocol")

local M = {}

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

return M
	`, WithStdlib(), WithModule("protocol", protocol))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want loop-guarded recursive route label to satisfy imported result return", result.Diagnostics)
	}
}
