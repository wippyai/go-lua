local uuid = { v4 = function(): (string?, string?) return "id", nil end }
local consts = {
    PROCESS_NAMES = { ORCHESTRATOR = "keeper.state.service.orchestrator" },
    TOPICS = { REGISTRY_CHANGE = "state.registry.change" },
}

local log = {
    debug = function(self, msg: string, fields: {[string]: any}?) end,
    info = function(self, msg: string, fields: {[string]: any}?) end,
    warn = function(self, msg: string, fields: {[string]: any}?) end,
    error = function(self, msg: string, fields: {[string]: any}?) end,
}

local function generate_channel_name()
    local id, err = uuid.v4()
    if err then
        log:error("Failed to generate UUID for channel name", {error = err})
        return nil, "Failed to generate UUID"
    end
    return "state.registry.response." .. id
end

local function run(args)
    log:debug("Registry watcher received notification")

    if not args or not args.result then
        log:warn("Invalid arguments provided to registry watcher")
        return {success = false, message = "Invalid arguments"}
    end

    local changeset = args.changeset or {}
    local result = args.result
    local request_id = args.request_id or "unknown"
    local user_id = args.user_id

    if not result.success then
        log:debug("Skipping processing for failed operation", {
            request_id = request_id,
            user_id = user_id,
            error = result.error or "Unknown error"
        })
        return {success = true}
    end

    local is_version_operation = #changeset == 0

    log:info("Processing registry changes", {
        changeset_count = #changeset,
        version = result.version,
        request_id = request_id,
        user_id = user_id,
        is_version_operation = is_version_operation
    })

    local response_channel_name, err = generate_channel_name()
    if not response_channel_name then
        log:error("Failed to generate response channel", {error = err})
        return {
            success = false,
            message = "Failed to generate response channel: " .. err
        }
    end

    local response_channel = process.listen(response_channel_name)

    local message = {
        changeset = changeset,
        version = result.version,
        timestamp = time.now():format("RFC3339Nano"),
        request_id = request_id,
        user_id = user_id,
        respond_to = response_channel_name,
        is_version_operation = is_version_operation
    }

    local ok = process.send(consts.PROCESS_NAMES.ORCHESTRATOR, consts.TOPICS.REGISTRY_CHANGE, message)
    if not ok then
        log:error("Failed to send registry change to orchestrator", {
            request_id = request_id
        })
        return {
            success = false,
            message = "Failed to send registry change to orchestrator"
        }
    end

    local timeout = time.after("30s")

    local select_result = channel.select({
        response_channel:case_receive(),
        timeout:case_receive()
    })

    if select_result.channel == timeout then
        log:error("Failed to sync registry changes to state - timeout", {
            request_id = request_id,
            user_id = user_id
        })
        return {
            success = false,
            message = "Failed to sync registry changes to state: timeout"
        }
    end

    local response = select_result.value

    if not response.success then
        log:error("Failed to sync registry changes to state", {
            request_id = request_id,
            user_id = user_id,
            error = response.error
        })
        return {
            success = false,
            message = "Failed to sync registry changes to state: " .. (response.error or "unknown error")
        }
    end

    log:info("Registry changes synced to state", {
        changeset_count = #changeset,
        changes_made = response.changes_made,
        version = response.version,
        request_id = request_id,
        is_version_operation = is_version_operation
    })

    return {success = true}
end

return {run = run}

