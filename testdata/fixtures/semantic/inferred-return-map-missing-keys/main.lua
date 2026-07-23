type Task = {kind: "task", id: string}
type Timer = {kind: "timer", id: string}
type Envelope = Task | Timer
type State = {processed: {[string]: Envelope}, counters: {[string]: number}}
type Actor = {state: State}

local function new_actor(): Actor
    return {state = {processed = {}, counters = {}}}
end

local actor = new_actor()
actor.state.processed["m1"] = {kind = "task", id = "m1"}
actor.state.counters["task"] = 1

local missing_processed: Envelope = actor.state.processed["missing"] -- expect-error
local missing_counter: number = actor.state.counters["missing"] -- expect-error

return "ok"
