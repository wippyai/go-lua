local http = require("http")
local security = require("security")
local component = require("component")
local reader = require("reader")
local writer = require("writer")
local types = require("types")

local M = {}

type ErrorBody = {
    success: boolean,
    error: string,
    code: string?,
    candidates: { unknown }?,
}

type ComponentMeta = {
    label: unknown?,
    title: unknown?,
    name: unknown?,
}

type ComponentRow = {
    component_id: unknown?,
    meta: unknown?,
}

type BootstrapProjection = {
    objects: unknown?,
    attributes: unknown?,
    pipelines: unknown?,
    views: unknown?,
}

local function trim(v: unknown): string
    if type(v) ~= "string" then return "" end
    return (v:gsub("^%s*(.-)%s*$", "%1"))
end

local function denied(message: string): ErrorBody
    return { success = false, error = message }
end

local function component_label(row: ComponentRow): string
    local meta: ComponentMeta =
        type(row.meta) == "table" and (row.meta :: ComponentMeta) or {}
    local label = trim(meta.label)
    if label == "" then label = trim(meta.title) end
    if label == "" then label = trim(meta.name) end
    if label == "" then label = "CRM" end
    return label
end

local function component_candidates(rows: { ComponentRow }): { unknown }
    local out: { unknown } = {}
    for _, row in ipairs(rows) do
        local id = trim(row.component_id)
        if id ~= "" then
            out[#out + 1] = {
                crm_id = id,
                component_id = id,
                label = component_label(row),
            }
        end
    end
    return out
end

local function workspace_label(crm_id: string): (string?, string?)
    local rows, err = component.query({
        component_ids = { crm_id },
        include = { meta = true },
        limit = 1,
    })
    if not rows then return nil, "CRM workspace metadata lookup failed: " .. tostring(err) end
    local row = type(rows[1]) == "table" and rows[1] :: ComponentRow or nil
    if not row then return nil, "CRM workspace metadata was not found" end
    local label = component_label(row)
    if label == "" then return nil, "CRM workspace title is missing" end
    return label, nil
end

function M.authorize(action: string): (number?, ErrorBody?)
    if not security.actor() then
        return http.STATUS.UNAUTHORIZED, denied("authentication required")
    end
    if security.can(action, "spiralscout.crm.api:*") == true
        or security.can("access", "spiralscout.crm.api:*") == true then
        return nil, nil
    end
    return http.STATUS.FORBIDDEN, denied("access denied")
end

function M.require_read(crm_id: string): (number?, ErrorBody?)
    local ok, err = reader.require_access(crm_id, component.ACCESS.READ)
    if not ok then return http.STATUS.FORBIDDEN, denied(tostring(err)) end
    return nil, nil
end

function M.require_write(crm_id: string): (number?, ErrorBody?)
    local ok, err = writer.require_access(crm_id, component.ACCESS.WRITE)
    if not ok then return http.STATUS.FORBIDDEN, denied(tostring(err)) end
    return nil, nil
end

function M.require_schema_admin(): (number?, ErrorBody?)
    if security.can("access", types.SCHEMA_MUTATION_GATE) == true then return nil, nil end
    return http.STATUS.FORBIDDEN, denied("CRM schema administration access denied")
end

function M.can_write(crm_id: string): boolean
    local ok = writer.require_access(crm_id, component.ACCESS.WRITE)
    return ok == true
end

function M.bootstrap_payload(crm_id: string): (unknown?, string?)
    local data, err = reader.bootstrap(crm_id)
    if not data then return nil, err end
    local label, label_err = workspace_label(crm_id)
    if not label then return nil, label_err end
    local bootstrap = data :: BootstrapProjection
    return {
        success = true,
        crm_id = crm_id,
        workspace_label = label,
        can_write = M.can_write(crm_id),
        objects = bootstrap.objects,
        attributes = bootstrap.attributes,
        pipelines = bootstrap.pipelines,
        views = bootstrap.views,
    }, nil
end

function M.resolve_bootstrap_crm_id(explicit_crm_id: string?): (string?, number?, ErrorBody?)
    local explicit = trim(explicit_crm_id)
    if explicit ~= "" then
        local status, body = M.require_read(explicit)
        if status then return nil, status, body end
        return explicit, nil, nil
    end

    local rows, err = component.query({
        impl_ids = { types.CRM_IMPL },
        access_mask = component.ACCESS.READ,
        include = { meta = true },
        order_by = { field = "created_at", direction = "ASC" },
        limit = 50,
    })
    if not rows then return nil, http.STATUS.INTERNAL_ERROR, denied(tostring(err)) end
    local components = rows :: { ComponentRow }
    if #components == 0 then
        return nil, http.STATUS.NOT_FOUND, {
            success = false,
            error = "crm component not found",
            code = "crm_not_provisioned",
        }
    end
    if #components > 1 then
        return nil, http.STATUS.CONFLICT, {
            success = false,
            error = "multiple crm components available",
            code = "multiple_crm_components",
            candidates = component_candidates(components),
        }
    end
    return trim(components[1].component_id), nil, nil
end

return M
