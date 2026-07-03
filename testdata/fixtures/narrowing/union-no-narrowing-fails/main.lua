type Event = {kind: string}
type Timer = {elapsed: number}
type Result = Event | Timer

function get_result(use_timer: boolean): Result
    if use_timer then
        return {elapsed = 1}
    end
    return {kind = "exit"}
end

function f(use_timer: boolean)
    local result = get_result(use_timer)
    local k: string = result.kind -- expect-error
end
