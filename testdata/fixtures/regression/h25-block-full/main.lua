local merge_arm = require("merge_arm")
local schema = require("schema")
local schema_authority = require("schema_authority")
local writer = require("writer")

local M = {}

type UnknownMap = { [string]: unknown }

local function declaration(): UnknownMap
    return {
        version = "block-probe-v1",
        objects = {
            { id = "entity-object", type = "entity", label = "Entity", plural = "Entities" },
        },
        attributes = {
            {
                id = "entity-name",
                object_type = "entity",
                attr = "name",
                data_type = "text",
                config = { searchable = true },
            },
        },
        pipelines = {},
        views = {},
        policies = {},
        merge_config = {},
    }
end

function M.run(args: unknown): (unknown?, string?)
    local crm_id = tostring(type(args) == "table" and (args :: UnknownMap).crm_id or "")
    local applied, apply_err = schema_authority.apply(crm_id, declaration()) -- expect-error: not enough arguments
    if not applied then return nil, apply_err end
    local left_ok, left_err = writer.create_record(crm_id, "entity:left", "entity", { name = "Left" })
    if not left_ok then return nil, left_err end
    local right_ok, right_err = writer.create_record(crm_id, "entity:right", "entity", { name = "Right" })
    if not right_ok then return nil, right_err end
    local projected, projection_err = writer.catch_up_read_model(crm_id)
    if not projected then return nil, projection_err end
    return merge_arm.preview_block({
        dataflow_id = "df:block-contract",
        node_id = "node:block-contract",
        signal_id = "signal:block-contract",
        input = {
            crm_id = crm_id,
            object_type = "entity",
            left_id = "entity:left",
            right_id = "entity:right",
            confidence = 0.91,
            reason = "matching identity",
        },
        config = { priority = "high" },
        run_context = {
            workflow_id = "workflow:block-contract",
            run_id = "run:block-contract",
        },
    })
end

return M

