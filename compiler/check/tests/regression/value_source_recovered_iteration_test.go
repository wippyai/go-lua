package regression

import "testing"

func TestRecoveredIterationDoesNotValidateDynamicState(t *testing.T) {
	checkModes(t, `
local function recover(payload, iteration)
    if not payload then return false, nil, nil, nil end
    local action_iteration = (payload.metadata and payload.metadata.iteration) or iteration or 0
    return false, nil, action_iteration, nil
end
local function use_number(x: number?) end
local function run(saved_state, payload)
    local iteration = saved_state.current_iteration or 0
    local complete, result, recovered_iteration, err = recover(payload, iteration)
    if recovered_iteration and recovered_iteration > iteration then
        iteration = recovered_iteration
    end
    use_number(iteration)
end
return run`, "", "expected number?, got any")
}

func TestRecoveredIterationArithmeticKeepsDynamicValue(t *testing.T) {
	checkModes(t, `
local function load_latest_agent_action(n)
    local actions = n:all()

    if actions and #actions > 0 then
        return actions[1]
    end

    return nil
end

local function load_action_payload(action_row)
    if not action_row then
        return nil
    end

    return {metadata = action_row.metadata or {}}

end

local function recover_persisted_action(n, iteration)
    local latest_action_row = load_latest_agent_action(n)
    if not latest_action_row then return false, nil, nil, nil end
    local action_payload = load_action_payload(latest_action_row)
    if not action_payload then return false, nil, nil, nil end
    local action_iteration = (action_payload.metadata and action_payload.metadata.iteration) or iteration or 0
    return false, nil, action_iteration, nil
end
local function want_n(x: number?) end
local function run(args, n)
    local saved_state = ((args.node or {}).metadata or {}).state or {}
    local iteration = saved_state.current_iteration or 0
    local recovered_complete, recovered_result, recovered_iteration = recover_persisted_action(n, iteration)
    if recovered_iteration and recovered_iteration > iteration then iteration = recovered_iteration end
    while iteration < (args.max_iterations or 10) and not recovered_complete do
        iteration = iteration + 1
        want_n(iteration)
    end
end
return run`, "", "expected number?, got any")
}

func TestRecoveredIterationValidatedNumbers(t *testing.T) {
	checkBothModes(t, `
local function recover(payload, iteration: number)
    if not payload then return false, nil, nil, nil end
    local action_iteration = tonumber((payload.metadata or {}).iteration) or iteration
    return false, nil, action_iteration, nil
end
local function use_number(x: number?) end
local function run(saved_state, payload)
    local iteration = tonumber(saved_state.current_iteration) or 0
    local complete, result, recovered_iteration, err = recover(payload, iteration)
    if recovered_iteration and recovered_iteration > iteration then
        iteration = recovered_iteration
    end
    while iteration < 10 and not complete do
        iteration = iteration + 1
        use_number(iteration)
    end
end
return run`, "")
}

func TestRecoveredIterationStringStillRejected(t *testing.T) {
	checkBothModes(t, `
local function recover() return false, nil, "bad", nil end
local function use_number(x: number?) end
local function run()
    local complete, result, recovered_iteration, err = recover()
    use_number(recovered_iteration)
end
return run`, "expected number?, got")
}
