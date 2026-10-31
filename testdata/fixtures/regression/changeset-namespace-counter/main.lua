local registry = require("registry")
local json = require("json")
local time = require("time")
local logger = require("logger")
local changelog_repo = require("changelog_repo")
local gov_consts = require("gov_consts")
local notify = require("notify")

local log = logger:named("gov.observer.changeset")

local CONST = {
    OPERATIONS = gov_consts.REGISTRY_OPERATIONS,

    -- Entity mappings - which kinds and meta.types to monitor for notifications
    ENTITY_MAPPINGS = {
        -- Map kinds to entry types
        KINDS = {
        },

        -- Map meta.types to entry types
        META_TYPES = {
            ["view.page"] = "pages",
            ["agent.gen1"] = "agents",
            ["llm.model"] = "models",
        }
    }
}

-- Helper function to determine entry type from registry entry
local function get_entry_type(entry)
    if not entry then return nil end

    -- Check kind mapping first
    if entry.kind and CONST.ENTITY_MAPPINGS.KINDS[entry.kind] then
        return CONST.ENTITY_MAPPINGS.KINDS[entry.kind]
    end

    -- Check meta.type mapping
    if entry.meta and entry.meta.type and CONST.ENTITY_MAPPINGS.META_TYPES[entry.meta.type] then
        return CONST.ENTITY_MAPPINGS.META_TYPES[entry.meta.type]
    end

    -- No mapping found
    return nil
end

-- Helper function to publish entry change event
local function publish_entry_change(entry_type, entry_id)
    local time_now = time.now():format("2006-01-02 15:04:05")

    -- Publish the entity change to subscribed admins via the events bus.
    notify.publish(entry_type, { id = entry_id })

    -- Also log a confirmation message
    log:debug("Entity change notification sent", {
        topic = entry_type,
        id = entry_id,
        time = time_now
    })
end

-- Main observer function
local function run(args)
    log:debug("Changeset observer received notification")

    -- Validate arguments
    if not args or not args.changeset or not args.result then
        log:warn("Invalid arguments provided to changeset observer")
        return {
            success = false,
            message = "Invalid arguments"
        }
    end

    local changeset = args.changeset
    local result = args.result
    local request_id = args.request_id or "unknown"
    local user_id = args.user_id

    -- Only process successful operations
    if not result.success then
        log:debug("Skipping processing for failed operation", {
            request_id = request_id,
            user_id = user_id,
            error = result.error or "Unknown error"
        })
        return {
            success = true
        }
    end

    -- Create a summary of the operations
    local summary = {
        timestamp = time.now():unix(),
        request_id = request_id,
        user_id = user_id,
        version = result.version,
        operation_counts = {
            create = 0,
            update = 0,
            delete = 0,
            total = #changeset
        },
        namespaces = {}
    }

    -- Process each operation for both logging and notifications
    for _, op in ipairs(changeset) do
        -- Count operations by type
        if op.kind == CONST.OPERATIONS.CREATE then
            summary.operation_counts.create = summary.operation_counts.create + 1
        elseif op.kind == CONST.OPERATIONS.UPDATE then
            summary.operation_counts.update = summary.operation_counts.update + 1
        elseif op.kind == CONST.OPERATIONS.DELETE then
            summary.operation_counts.delete = summary.operation_counts.delete + 1
        end

        -- Extract and count by namespace
        if op.entry and op.entry.id then
            local namespace = op.entry.id:match("(.+):.+")
            if namespace then
                if not summary.namespaces[namespace] then
                    summary.namespaces[namespace] = {
                        create = 0,
                        update = 0,
                        delete = 0
                    }
                end

                if op.kind == CONST.OPERATIONS.CREATE then
                    summary.namespaces[namespace].create = summary.namespaces[namespace].create + 1
                elseif op.kind == CONST.OPERATIONS.UPDATE then
                    summary.namespaces[namespace].update = summary.namespaces[namespace].update + 1
                elseif op.kind == CONST.OPERATIONS.DELETE then
                    summary.namespaces[namespace].delete = summary.namespaces[namespace].delete + 1
                end
            end

            -- Check if this entry should trigger notifications
            local entry_type = get_entry_type(op.entry)
            if entry_type then
                publish_entry_change(entry_type, op.entry.id)
            elseif op.kind == CONST.OPERATIONS.DELETE then
                -- For delete operations without specific mapping, use generic fallback
                publish_entry_change("registry", op.entry.id)
            end
        end
    end

    -- Publish the registry version update to subscribed admins.
    if result.version then
        notify.publish("registry:version", {
            version = result.version,
            operation_counts = summary.operation_counts,
            namespaces = summary.namespaces,
            timestamp = summary.timestamp,
            user_id = user_id
        })

        log:debug("Registry version update notification sent", {
            version = result.version,
            operations = summary.operation_counts.total
        })
    end

    -- Persist to changelog
    local cl_ok, cl_err = pcall(function()
        changelog_repo.record_changeset(args)
    end)
    if not cl_ok then
        log:warn("Failed to persist changelog", { error = cl_err })
    end

    -- Log the summary
    log:info("Registry change summary", summary)

    log:info("Changeset processing complete", {
        request_id = request_id,
        user_id = user_id,
        operations_processed = summary.operation_counts.total,
        time = time.now():format("2006-01-02 15:04:05")
    })

    return {
        success = true
    }
end

return { run = run }
