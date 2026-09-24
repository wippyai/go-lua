local function create_array(size)
    return table.create(math.floor(size or 0), 0)
end

local function add_failure(parallel_result, iteration: any, err_message)
    parallel_result.failure_count = parallel_result.failure_count + 1
    parallel_result.failures[parallel_result.failure_count] = {
        iteration = iteration.iteration or iteration.iteration_index,
        item = iteration.input_item,
        error = err_message
    }
end

local function add_success(parallel_result, iteration: any, result)
    parallel_result.success_count = parallel_result.success_count + 1
    parallel_result.successes[parallel_result.success_count] = {
        iteration = iteration.iteration or iteration.iteration_index,
        item = iteration.input_item,
        result = result
    }
end

local function create_parallel_result(items)
    return {
        successes = create_array(#items),
        failures = create_array(#items),
        success_count = 0,
        failure_count = 0,
        total_iterations = #items
    }
end

local function copy_parallel_entries(entries, entry_count)
    local copied = {}
    if type(entries) ~= "table" then
        return copied
    end

    local max_index = type(entry_count) == "number" and entry_count or #entries
    for index = 1, max_index do
        local entry = entries[index]
        if entry ~= nil then
            copied[#copied + 1] = entry
        end
    end

    return copied
end

local function collect_all_parallel_entries(parallel_result)
    local ordered = {}
    local entries_by_iteration = {}

    for index = 1, (parallel_result.success_count or 0) do
        local entry = parallel_result.successes[index]
        if type(entry) == "table" and type(entry.iteration) == "number" then
            entries_by_iteration[entry.iteration] = entry
        end
    end

    for index = 1, (parallel_result.failure_count or 0) do
        local entry = parallel_result.failures[index]
        if type(entry) == "table" and type(entry.iteration) == "number" then
            entries_by_iteration[entry.iteration] = entry
        end
    end

    for iteration = 1, (parallel_result.total_iterations or 0) do
        local entry = entries_by_iteration[iteration]
        if entry ~= nil then
            ordered[#ordered + 1] = entry
        end
    end

    return ordered
end

local function run(items)
    local parallel_result = create_parallel_result(items)
    for index, item in ipairs(items) do
        local iteration = { iteration = index, input_item = item }
        if item.ok then
            add_success(parallel_result, iteration, item.value)
        else
            add_failure(parallel_result, iteration, "failed")
        end
    end
    local successes = copy_parallel_entries(parallel_result.successes, parallel_result.success_count)
    local failures = copy_parallel_entries(parallel_result.failures, parallel_result.failure_count)
    local all = collect_all_parallel_entries(parallel_result)
    return #successes, #failures, #all
end

local s, f, a = run({ { ok = true, value = 1 }, { ok = false } })
assert(s == 1 and f == 1 and a == 2)
