-- dataflow/src/node/parallel/parallel.lua: add_failure only stores its message,
-- so a failure whose error is unknown is accepted like one with a string.
local PARALLEL_PROGRESS_OUTCOME = { SUCCESS = "success", FAILURE = "failure", FILTERED = "filtered" }
local FAIL_FAST = "fail_fast"

local parallel = {
    _deps = {
        iterator = {
            collect_results = function(n: any, iteration: any): (any, unknown)
                return nil, nil
            end,
        },
    },
}

local function prefixed_error(prefix: string, err: any, fallback: string): string
    if err == nil then
        return prefix .. fallback
    end
    return prefix .. tostring(err)
end

local function add_failure(parallel_result, iteration: any, err_message)
    parallel_result.failure_count = parallel_result.failure_count + 1
    parallel_result.failures[parallel_result.failure_count] = {
        iteration = iteration.iteration or iteration.iteration_index,
        item = iteration.input_item,
        error = err_message
    }
end

local function build_iteration_completion(iteration: any, outcome, extras)
    extras = extras or {}

    return {
        iteration = iteration.iteration,
        outcome = outcome,
        attempt_id = extras.attempt_id or iteration.attempt_id,
        result = extras.result,
        error = extras.error
    }
end

local function apply_iteration_completion(parallel_result, iteration: any, completion: any)
    if completion.outcome == PARALLEL_PROGRESS_OUTCOME.FAILURE then
        add_failure(parallel_result, iteration, completion.error or "Iteration failed")
    end
end

local function collect_iteration_completion(n, failure_strategy, parallel_result, iteration: any)
    local iteration_result, collect_err = parallel._deps.iterator.collect_results(n, iteration)
    if collect_err then
        if failure_strategy == FAIL_FAST then
            return nil, prefixed_error("Iteration failed: ", collect_err, "unknown")
        end

        local completion = build_iteration_completion(iteration, PARALLEL_PROGRESS_OUTCOME.FAILURE, {
            error = collect_err
        })
        add_failure(parallel_result, iteration, completion.error)
        return completion, nil
    end

    local pipeline_err = prefixed_error("Item pipeline failed: ", iteration_result, "unknown")
    add_failure(parallel_result, iteration, pipeline_err)
    return build_iteration_completion(iteration, PARALLEL_PROGRESS_OUTCOME.SUCCESS, { result = iteration_result }), nil
end

local result = { failure_count = 0, failures = {} }
apply_iteration_completion(result, { iteration = 1 }, { outcome = "failure" })
return collect_iteration_completion({}, FAIL_FAST, result, { iteration = 2 })
