local aliases = require("aliases")
local materializer = require("materializer")
local sql = require("sql")
local storage = require("storage")
local test = require("test")
local types = require("types")
local uuid = require("uuid")
local write_origin = require("write_origin")

type UnknownMap = { [string]: unknown }
type UnknownList = { unknown }
type Attribute = {
    id: string,
    object_type: string,
    attr: string,
    data_type: string,
    config: UnknownMap,
}
type ObjectDeclaration = { id: string, type: string, label: string, plural: string? }
type Declaration = {
    version: string,
    objects: { ObjectDeclaration },
    attributes: { Attribute },
    pipelines: UnknownList,
    views: UnknownList,
    policies: UnknownList,
    merge_config: UnknownMap,
    remove: UnknownMap?,
}
type Record = { record_id: string, values: UnknownMap }
type SchemaObject = { table_name: string?, relations: { [string]: unknown } }
type SchemaDeclaration = {
    version: string,
    pipelines: UnknownList,
    views: UnknownList,
    policies: UnknownList,
    merge_config: UnknownMap,
}
type Schema = {
    revision: unknown,
    source_hash: unknown,
    declaration: SchemaDeclaration,
    objects: { [string]: SchemaObject },
}
type ApplyResult = { changed: boolean }
type ListResult = { total: number, rows: { Record } }
type TypedRef = { kind: string, type: string, id: string }
type FieldResult = { value: unknown }
type RelationEdge = { source: TypedRef, target: TypedRef }
type Related = { incoming: { RelationEdge } }
type RelationGroups = { [string]: { outgoing: { RelationEdge }, incoming: { RelationEdge } } }
type WriteOrigin = { found: boolean, write_ref: string? }
type AliasClaim = { record_id: string, state: string, won: boolean }
type Event = { type: string, body: UnknownMap }

local function require_map(value: unknown, name: string): UnknownMap
    if type(value) ~= "table" then error(name .. " must be a table", 0) end
    return value :: UnknownMap
end

local function require_list(value: unknown, name: string): UnknownList
    local list = require_map(value, name)
    return list :: UnknownList
end

local function require_string(value: unknown, name: string): string
    if type(value) ~= "string" then error(name .. " must be a string", 0) end
    return value
end

local function require_number(value: unknown, name: string): number
    if type(value) ~= "number" then error(name .. " must be a number", 0) end
    return value
end

local function require_boolean(value: unknown, name: string): boolean
    if type(value) ~= "boolean" then error(name .. " must be a boolean", 0) end
    return value
end

local function require_record(value: unknown): Record
    local map = require_map(value, "record")
    return {
        record_id = require_string(map.record_id, "record.record_id"),
        values = require_map(map.values, "record.values"),
    }
end

local function require_schema(value: unknown): Schema
    local map = require_map(value, "schema")
    local declaration = require_map(map.declaration, "schema.declaration")
    return {
        revision = map.revision,
        source_hash = map.source_hash,
        declaration = {
            version = require_string(declaration.version, "schema.declaration.version"),
            pipelines = require_list(declaration.pipelines, "schema.declaration.pipelines"),
            views = require_list(declaration.views, "schema.declaration.views"),
            policies = require_list(declaration.policies, "schema.declaration.policies"),
            merge_config = require_map(declaration.merge_config, "schema.declaration.merge_config"),
        },
        objects = require_map(map.objects, "schema.objects") :: { [string]: SchemaObject },
    }
end

local function require_apply_result(value: unknown): ApplyResult
    local map = require_map(value, "schema apply result")
    return { changed = require_boolean(map.changed, "schema apply result.changed") }
end

local function require_list_result(value: unknown): ListResult
    local map = require_map(value, "record list")
    local rows: { Record } = {}
    for _, raw_row in ipairs(require_list(map.rows, "record list.rows")) do
        rows[#rows + 1] = require_record(raw_row)
    end
    return { total = require_number(map.total, "record list.total"), rows = rows }
end

local function require_ref(value: unknown, name: string): TypedRef
    local map = require_map(value, name)
    return {
        kind = require_string(map.kind, name .. ".kind"),
        type = require_string(map.type, name .. ".type"),
        id = require_string(map.id, name .. ".id"),
    }
end

local function require_refs(value: unknown, name: string): { TypedRef }
    local refs: { TypedRef } = {}
    for _, raw_ref in ipairs(require_list(value, name)) do
        refs[#refs + 1] = require_ref(raw_ref, name .. " item")
    end
    return refs
end

local function require_field(value: unknown): FieldResult
    return { value = require_map(value, "field").value }
end

local function require_related(value: unknown): Related
    local map = require_map(value, "related")
    local incoming = require_list(map.incoming, "related.incoming")
    local edges: { RelationEdge } = {}
    for _, item in ipairs(incoming) do
        local edge = require_map(item, "related edge")
        edges[#edges + 1] = {
            source = require_ref(edge.source, "related edge.source"),
            target = require_ref(edge.target, "related edge.target"),
        }
    end
    return { incoming = edges }
end

local function require_relation_groups(value: unknown): RelationGroups
    local map = require_map(value, "related records")
    local groups: RelationGroups = {}
    for record_id, raw_group in pairs(map) do
        local group = require_map(raw_group, "relation group")
        local function edges(raw: unknown, name: string): { RelationEdge }
            local output: { RelationEdge } = {}
            for _, raw_edge in ipairs(require_list(raw, name)) do
                local edge = require_map(raw_edge, name .. " edge")
                output[#output + 1] = {
                    source = require_ref(edge.source, name .. " edge.source"),
                    target = require_ref(edge.target, name .. " edge.target"),
                }
            end
            return output
        end
        groups[record_id] = { outgoing = edges(group.outgoing, "relation group.outgoing"), incoming = edges(group.incoming, "relation group.incoming") }
    end
    return groups
end

local function require_write_origin(value: unknown): WriteOrigin
    local map = require_map(value, "write origin")
    local write_ref = map.write_ref
    return {
        found = require_boolean(map.found, "write origin.found"),
        write_ref = type(write_ref) == "string" and write_ref or nil,
    }
end

local function require_alias_claim(value: unknown): AliasClaim
    local map = require_map(value, "alias claim")
    return {
        record_id = require_string(map.record_id, "alias claim.record_id"),
        state = require_string(map.state, "alias claim.state"),
        won = require_boolean(map.won, "alias claim.won"),
    }
end

local function db(): (sql.DB, string)
    local handle, err = sql.get(types.db_id())
    if err then error(tostring(err), 0) end
    local db_type, type_err = handle:type()
    if type_err then handle:release(); error(tostring(type_err), 0) end
    if db_type == sql.type.SQLITE then return handle, "sqlite" end
    if db_type == sql.type.POSTGRES then return handle, "postgres" end
    handle:release()
    error("unsupported test database", 0)
end

local function declaration(version: string, name_attr: string?): Declaration
    return {
        version = version,
        objects = { { id = "entity-object", type = "entity", label = "Entity", plural = "Entities" } },
        attributes = {
            {
                id = "entity-name",
                object_type = "entity",
                attr = name_attr or "name",
                data_type = "text",
                config = { searchable = true },
            },
            {
                id = "entity-email",
                object_type = "entity",
                attr = "email",
                data_type = "email",
                config = { alias = "email", searchable = true },
            },
            {
                id = "entity-score",
                object_type = "entity",
                attr = "score",
                data_type = "number",
                config = { indexed = true },
            },
            {
                id = "entity-active",
                object_type = "entity",
                attr = "active",
                data_type = "boolean",
                config = {},
            },
            {
                id = "entity-parent",
                object_type = "entity",
                attr = "parent",
                data_type = "relation",
                config = {
                    cardinality = "one",
                    target = { kind = "record", object_type = "entity" },
                },
            },
            {
                id = "entity-peers",
                object_type = "entity",
                attr = "peers",
                data_type = "relation",
                config = {
                    cardinality = "many",
                    target = { kind = "record", object_type = "entity" },
                },
            },
        },
        pipelines = {},
        views = {},
        policies = {},
        merge_config = {},
    }
end

local function relation_declaration(version: string): Declaration
    return {
        version = version,
        objects = {
            { id = "company-object", type = "company", label = "Company" },
            { id = "contact-object", type = "contact", label = "Contact" },
        },
        attributes = {
            { id = "company-name", object_type = "company", attr = "name", data_type = "text", config = { searchable = true } },
            { id = "contact-name", object_type = "contact", attr = "name", data_type = "text", config = { searchable = true } },
            { id = "contact-company", object_type = "contact", attr = "company", data_type = "relation",
                config = { cardinality = "one", target = { kind = "record", object_type = "company" } } },
        },
        pipelines = { { collection_id = "company_pipeline", object_type = "company", label = "Company pipeline", stages = {} } },
        views = { { view_id = "company_table", object_type = "company", name = "Companies", layout = "table" } },
        policies = { { key = "retained", value = true } },
        merge_config = { auto_merge = false },
    }
end

local function unique_declaration(version: string, enabled: boolean): Declaration
    local wanted = declaration(version)
    for _, field in ipairs(wanted.attributes) do
        if field.attr == "email" then field.config.unique = enabled end
    end
    return wanted
end

local function wide_declaration(): Declaration
    local wanted = {
        version = "wide-v1",
        objects = { { id = "wide-object", type = "wide", label = "Wide" } },
        attributes = {},
        pipelines = {},
        views = {},
        policies = {},
        merge_config = {},
    }
    for index = 1, 300 do
        wanted.attributes[#wanted.attributes + 1] = {
            id = string.format("wide-field-%03d", index),
            object_type = "wide",
            attr = string.format("field_%03d", index),
            data_type = "text",
            config = { searchable = false },
        }
    end
    return wanted
end
