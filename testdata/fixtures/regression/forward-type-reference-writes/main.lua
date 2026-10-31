type Runtime = {
    nudges: { [string]: Nudge },
    known: { [string]: boolean },
    bootstrapped: boolean,
    epoch: string?,
}

type Nudge = {
    dataflow_id: string,
    generation: number,
    wake_key: string?,
    wake_at: string?,
}


local M = {}

function M.new_runtime(epoch: string?): Runtime
    return {
        nudges = {},
        known = {},
        bootstrapped = false,
        epoch = epoch,
    }
end

function M.reconcile_activation(runtime: Runtime, dataflow_id: string, generation: number,
    desired_active: boolean, nudge: Nudge?): (boolean?, string?)
    if not runtime.epoch then return nil, "runtime epoch is unavailable" end
    runtime.known[dataflow_id] = true
    if desired_active then
        runtime.nudges[dataflow_id] = nudge or {
            dataflow_id = dataflow_id,
            generation = generation,
        }
    else
        runtime.nudges[dataflow_id] = nil
    end
    return true, nil
end

function M.handle_activation_hint(runtime: Runtime, payload: any): (boolean?, string?)
    return M.reconcile_activation(runtime, tostring(payload.dataflow_id), 1, payload.active == true)
end

function M.bootstrap(runtime: Runtime): (number?, string?)
    runtime.bootstrapped = true
    return 0, nil
end

local function reconcile_or_log(runtime: Runtime, operation: (Runtime) -> (any?, string?))
    local ok, err = operation(runtime)
    if not ok and err then
        runtime.bootstrapped = false
    end
end

function M.run(payloads: { any }): Runtime
    local runtime = M.new_runtime("epoch-1")
    reconcile_or_log(runtime, M.bootstrap)
    for _, payload in ipairs(payloads) do
        if payload.direct then
            M.handle_activation_hint(runtime, payload)
        elseif not runtime.bootstrapped then
            reconcile_or_log(runtime, M.bootstrap)
        else
            local function targeted(current: Runtime)
                return M.handle_activation_hint(current, payload)
            end
            reconcile_or_log(runtime, targeted)
        end
    end
    return runtime
end

local runtime = M.run({ { dataflow_id = "d1", active = true }, { dataflow_id = "d2", active = true, direct = true } })
assert(runtime.nudges.d1 and runtime.nudges.d1.generation == 1)
