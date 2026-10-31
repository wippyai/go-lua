-- Compiles declared CRM schemas into engine-specific SQL plans.
local hash = require("hash")
local sql = require("sql")

local M = {}
local MAX_ID = 63
type JsonMap = { [string]: unknown }
type DynamicTable = { [string | number]: unknown }
type Executor = sql.DB | sql.Transaction
type SqlParams = { unknown }
type SqlRow = { [string]: unknown }
type SqlRows = { SqlRow }
type TargetConfig = {
    kind: string?,
    object_type: string?,
    class: string?,
    kinds: { unknown }?,
}
type FieldConfig = {
    stable_id: string?,
    target: TargetConfig?,
    cardinality: string?,
    unique: boolean?,
    indexed: boolean?,
    alias: unknown?,
    searchable: boolean?,
    -- The closed vocabulary a select/multi_select is bounded by; required for
    -- those types at compile time.
    choices: { unknown }?,
}
type ObjectSource = {
    id: string?,
    type: string,
    object_id: string,
    explicit_id: boolean,
    label: string?,
    icon: string?,
    plural: string?,
    is_syncable: boolean?,
    config: FieldConfig?,
}
type FieldSource = {
    id: string?,
    object_type: string,
    object_id: string?,
    field_id: string,
    explicit_id: boolean,
    attr: string,
    data_type: string,
    label: string?,
    config: FieldConfig,
    sort_order: number?,
    system: boolean?,
    target_object_id: string?,
}
type CompiledField = {
    object_id: string,
    object_type: string,
    field_id: string,
    explicit_id: boolean,
    attr: string,
    data_type: string,
    label: string?,
    config: FieldConfig,
    sort_order: number,
    system: boolean,
    definition: string,
    column_name: string?,
    relation_table: string?,
    target_kind: string?,
    target_type: string?,
    target_object_id: string?,
    cardinality: string?,
    indexed: boolean,
    searchable: boolean,
    sql_type: string?,
}
type CompiledObject = {
    crm_id: string?,
    object_id: string,
    explicit_id: boolean,
    object_type: string,
    label: string?,
    icon: string?,
    plural: string?,
    is_syncable: boolean,
    definition: string,
    table_name: string,
    fts_table: string?,
    fields: { CompiledField },
    relations: { CompiledField },
}
type Normalized = {
    version: unknown?,
    merge_config: JsonMap,
    objects: { ObjectSource },
    attributes: { FieldSource },
    pipelines: { JsonMap },
    views: { JsonMap },
    policies: { JsonMap },
    remove: JsonMap,
}
type Plan = {
    crm_id: string,
    dialect: string,
    revision: string,
    source_hash: string,
    manifest: string,
    declaration: Normalized,
    objects: { CompiledObject },
}
type ApplyResult = {
    revision: string,
    source_hash: string,
    changed: boolean,
    objects: { CompiledObject },
}
local SCALAR = {
    text = true,
    number = true,
    boolean = true,
    date = true,
    time = true,
    datetime = true,
    select = true,
    multi_select = true,
    currency = true,
    email = true,
    phone = true,
    url = true,
    ai = true,
}
local SEARCHABLE = {
    text = true,
    select = true,
    email = true,
    phone = true,
    url = true,
    ai = true,
}

local function trim(v: unknown): string
    return type(v) == "string" and v:match("^%s*(.-)%s*$") or ""
end

local function clone(v: unknown): unknown
    if type(v) ~= "table" then
        return v
    end
    local source = v :: DynamicTable
    local out: DynamicTable = {}
    for k, item in pairs(source) do
        out[k] = clone(item)
    end
    return out
end

local function stable(v: unknown): string
    local t = type(v)
    if t == "nil" then
        return "null"
    end
    if t == "boolean" then
        return v and "true" or "false"
    end
    if t == "number" then
        return tostring(v)
    end
    if t == "string" then
        return '"' .. v:gsub("\\", "\\\\"):gsub('"', '\\"'):gsub("\n", "\\n"):gsub("\r", "\\r"):gsub("\t", "\\t") .. '"'
    end
    if t ~= "table" then
        error("unsupported declaration value: " .. t)
    end
    local source = v :: DynamicTable
    local n, array = #source, #source > 0
    if array then
        for k in pairs(source) do
            if type(k) ~= "number" or k < 1 or k > n or k % 1 ~= 0 then
                array = false
                break
            end
        end
    end
    local out = {}
    if array then
        for i = 1, n do
            out[i] = stable(source[i])
        end
        return "[" .. table.concat(out, ",") .. "]"
    end
    local keys = {}
    for k in pairs(source) do
        keys[#keys + 1] = tostring(k)
    end
    table.sort(keys)
    for _, k in ipairs(keys) do
        out[#out + 1] = stable(k) .. ":" .. stable(source[k])
    end
    return "{" .. table.concat(out, ",") .. "}"
end

local function digest(v: unknown): string
    return tostring(hash.sha256(v)):gsub("[^a-fA-F0-9]", ""):lower():sub(1, 12)
end

local function slug(v: unknown): string
    local s = trim(v):lower():gsub("[^a-z0-9]+", "_"):gsub("^_+", ""):gsub("_+$", "")
    if s == "" then
        s = "x"
    end
    if s:match("^[0-9]") then
        s = "x_" .. s
    end
    return s
end

local function identifier(prefix: string, logical: unknown): string
    local suffix = digest(logical)
    local room = MAX_ID - #prefix - #suffix - 1
    if room < 1 then
        error("identifier prefix too long")
    end
    return prefix .. slug(logical):sub(1, room) .. "_" .. suffix
end

local function ident(v: unknown): string
    if type(v) ~= "string" or #v > MAX_ID or not v:match("^[a-z][a-z0-9_]*$") then
        error("invalid catalog identifier")
    end
    return v :: string
end

local function exec(db: Executor, sql: string, params: SqlParams?)
    local _, err = db:execute(tostring(sql), params or {})
    if err then
        error(tostring(err))
    end
end

local function rows(db: Executor, sql: string, params: SqlParams?): SqlRows
    local out, err = db:query(tostring(sql), params or {})
    if err then
        error(tostring(err))
    end
    return (out or {}) :: SqlRows
end

local function bind(params: SqlParams, value: unknown): string
    if value == nil then
        return "NULL"
    end
    params[#params + 1] = value
    return "$" .. tostring(#params)
end

local function bool_type(d: string): string
    return d == "postgres" and "BOOLEAN" or "INTEGER"
end

local function time_type(d: string): string
    return d == "postgres" and "TIMESTAMPTZ" or "TEXT"
end

local function sql_type(t: unknown, d: string): string
    if t == "number" or t == "currency" then
        return d == "postgres" and "DOUBLE PRECISION" or "REAL"
    end
    if t == "boolean" then
        return bool_type(d)
    end
    if t == "date" then
        return d == "postgres" and "DATE" or "TEXT"
    end
    if t == "time" then
        return d == "postgres" and "TIME" or "TEXT"
    end
    if t == "datetime" then
        return time_type(d)
    end
    if t == "multi_select" then
        return d == "postgres" and "JSONB" or "TEXT"
    end
    return "TEXT"
end

local function explicit_id(item: { id: string?, config: FieldConfig? }): string
    return trim(item.id) ~= "" and trim(item.id) or trim(type(item.config) == "table" and item.config.stable_id or nil)
end

local function sequence(value: unknown, name: string): { unknown }
    if value == nil then
        return {}
    end
    if type(value) ~= "table" then
        error(name .. " must be an array")
    end
    local length = #value
    for key in pairs(value) do
        if type(key) ~= "number" or key % 1 ~= 0 or key < 1 or key > length then
            error(name .. " must be a dense array")
        end
    end
    return value :: { unknown }
end

local function normalize(declaration: unknown): Normalized
    if type(declaration) ~= "table" then
        error("CRM schema declaration is required")
    end
    local source_declaration = declaration :: JsonMap
    if source_declaration.merge_config ~= nil and type(source_declaration.merge_config) ~= "table" then
        error("merge_config must be a map")
    end
    if source_declaration.remove ~= nil and type(source_declaration.remove) ~= "table" then
        error("remove must be a map")
    end
    local out: Normalized = {
        version = source_declaration.version,
        merge_config = clone(source_declaration.merge_config or {}) :: JsonMap,
        objects = {},
        attributes = {},
        pipelines = clone(sequence(source_declaration.pipelines, "pipelines")) :: { JsonMap },
        views = clone(sequence(source_declaration.views, "views")) :: { JsonMap },
        policies = clone(sequence(source_declaration.policies, "policies")) :: { JsonMap },
        remove = clone(source_declaration.remove or {}) :: JsonMap,
    }
    local object_types, object_ids = {}, {}
    for _, source in ipairs(sequence(source_declaration.objects, "objects")) do
        if type(source) ~= "table" then
            error("objects must contain maps")
        end
        local object = clone(source) :: ObjectSource
        object.type = trim(object.type)
        if object.type == "" then
            error("object type is required")
        end
        local declared_id = explicit_id(object)
        object.object_id = declared_id ~= "" and declared_id or object.type
        object.explicit_id = declared_id ~= ""
        if object_types[object.type] then
            error("duplicate object type: " .. object.type)
        end
        if object_ids[object.object_id] then
            error("duplicate object stable id: " .. object.object_id)
        end
        object_types[object.type] = object.object_id
        object_ids[object.object_id] = true
        out.objects[#out.objects + 1] = object
    end
    table.sort(out.objects, function(a, b)
        return a.object_id < b.object_id
    end)
    local field_ids, logical_fields = {}, {}
    for _, source in ipairs(sequence(source_declaration.attributes, "attributes")) do
        if type(source) ~= "table" then
            error("attributes must contain maps")
        end
        local field = clone(source) :: FieldSource
        field.object_type = trim(field.object_type)
        field.attr = trim(field.attr)
        field.data_type = trim(field.data_type)
        if not object_types[field.object_type] then
            error("attribute references unknown object: " .. field.object_type)
        end
        if field.attr == "" or (field.data_type ~= "relation" and not SCALAR[field.data_type]) then
            error("invalid attribute: " .. field.object_type .. "." .. field.attr)
        end
        if field.config ~= nil and type(field.config) ~= "table" then
            error("attribute config must be a map: " .. field.object_type .. "." .. field.attr)
        end
        field.config = type(field.config) == "table" and field.config or {}
        -- A select is a closed vocabulary; without one it silently degrades to
        -- an unbounded text column that the compiler still auto-indexes -- and
        -- a single long value then dead-letters the whole read model on the
        -- btree entry cap. The invariant is enforced where the type is
        -- declared: a choice-typed attribute names its choices, and every
        -- choice is bounded, so an auto-indexed column is bounded by
        -- construction. An open set of values is a text attribute.
        if field.data_type == "select" or field.data_type == "multi_select" then
            local declared = field.config.choices
            if type(declared) ~= "table" or #(declared :: { unknown }) == 0 then
                error("a " .. field.data_type .. " attribute requires a non-empty config.choices vocabulary: "
                    .. field.object_type .. "." .. field.attr .. " (an open value set is a text attribute)")
            end
            for _, choice in ipairs(declared :: { unknown }) do
                local token = type(choice) == "table" and tostring((choice :: { value: unknown }).value or "")
                    or tostring(choice or "")
                if token == "" then
                    error("a choice must carry a non-empty value: " .. field.object_type .. "." .. field.attr)
                end
                if #token > 255 then
                    error("a choice value must stay within 255 bytes: " .. field.object_type .. "." .. field.attr)
                end
            end
        end
        local declared_id = explicit_id(field)
        field.field_id = declared_id ~= "" and declared_id or field.attr
        field.explicit_id = declared_id ~= ""
        local object: ObjectSource? = nil
        for _, candidate in ipairs(out.objects) do
            if candidate.type == field.object_type then
                object = candidate
                break
            end
        end
        local ikey, lkey = object.object_id .. "\0" .. field.field_id, field.object_type .. "\0" .. field.attr
        if field_ids[ikey] or logical_fields[lkey] then
            error("duplicate attribute identity: " .. field.object_type .. "." .. field.attr)
        end
        field_ids[ikey], logical_fields[lkey] = true, true
        if field.data_type == "relation" then
            if field.config.unique == true then
                error("relation uniqueness is expressed by cardinality: " .. field.object_type .. "." .. field.attr)
            end
            local target: TargetConfig? = type(field.config.target) == "table" and field.config.target or nil
            local kind = target and trim(target.kind) or ""
            if kind ~= "record" and kind ~= "principal" and kind ~= "component" then
                error("relation target kind is required")
            end
            if kind == "record" and not object_types[trim(target.object_type)] then
                error("unknown relation object target: " .. tostring(target.object_type))
            end
            if kind == "record" then
                field.target_object_id = object_types[trim(target.object_type)]
            end
            if kind == "component" and trim(target.class) == "" then
                error("component relation target class is required")
            end
            if kind == "principal" then
                local kinds = sequence(target.kinds, "principal relation target kinds")
                if #kinds == 0 then
                    error("principal relation target kinds are required")
                end
                for _, principal_kind in ipairs(kinds) do
                    if trim(principal_kind) == "" then
                        error("principal relation target kinds must be strings")
                    end
                end
            end
            local cardinality = trim(field.config.cardinality)
            if cardinality == "" then
                cardinality = "one"
            end
            if cardinality ~= "one" and cardinality ~= "many" then
                error("relation cardinality must be one or many")
            end
            field.config.cardinality = cardinality
        end
        out.attributes[#out.attributes + 1] = field
    end
    table.sort(out.attributes, function(a, b)
        local ak = a.object_type .. "\0" .. a.field_id
        local bk = b.object_type .. "\0" .. b.field_id
        return ak < bk
    end)
    for _, name in ipairs({ "pipelines", "views", "policies" }) do
        for _, item in ipairs(out[name]) do
            if type(item) ~= "table" then
                error(name .. " must contain maps")
            end
        end
        table.sort(out[name], function(a, b)
            return stable(a) < stable(b)
        end)
    end
    local declaration_ids = {}
    for _, item in ipairs(out.pipelines) do
        local id = trim(item.collection_id)
        if id == "" or declaration_ids["pipeline\0" .. id] then
            error("pipeline declaration id is missing or duplicated")
        end
        declaration_ids["pipeline\0" .. id] = true
    end
    for _, item in ipairs(out.views) do
        local id = trim(item.view_id)
        if id == "" or declaration_ids["view\0" .. id] then
            error("view declaration id is missing or duplicated")
        end
        declaration_ids["view\0" .. id] = true
    end
    for _, item in ipairs(out.policies) do
        local id = trim(item.portableKey) ~= "" and trim(item.portableKey) or trim(item.key)
        if id == "" or declaration_ids["policy\0" .. id] then
            error("policy declaration id is missing or duplicated")
        end
        declaration_ids["policy\0" .. id] = true
    end
    for _, name in ipairs({ "attributes", "objects", "declarations" }) do
        if out.remove[name] ~= nil then
            sequence(out.remove[name], "remove." .. name)
        end
    end
    return out
end

local function relation_target(field: FieldSource): (string, string, string?)
    local target = field.config.target
    if target.kind == "record" then
        return "record", trim(target.object_type), field.target_object_id
    end
    if target.kind == "component" then
        return "component", trim(target.class)
    end
    local kinds = clone(target.kinds)
    table.sort(kinds)
    return "principal", table.concat(kinds, ",")
end

local function compile_field(crm_id: string, object: ObjectSource, field: FieldSource, dialect: string): CompiledField
    local logical = crm_id .. "\0" .. object.object_id .. "\0" .. field.field_id
    local authored = clone(field)
    authored.field_id = nil
    authored.explicit_id = nil
    authored.target_object_id = nil
    local common: CompiledField = {
        object_id = object.object_id,
        object_type = object.type,
        field_id = field.field_id,
        explicit_id = field.explicit_id,
        attr = field.attr,
        data_type = field.data_type,
        label = field.label,
        config = field.config,
        sort_order = tonumber(field.sort_order) or 0,
        system = field.system ~= false,
        definition = stable(authored),
        indexed = false,
        searchable = false,
    }
    if field.data_type == "relation" then
        common.relation_table = identifier("spiralscout_crm_rel_", logical)
        common.target_kind, common.target_type, common.target_object_id = relation_target(field)
        common.cardinality = field.config.cardinality
        common.indexed = true
        common.searchable = false
        return common
    end
    common.column_name = identifier("f_", logical)
    common.sql_type = sql_type(field.data_type, dialect)
    common.indexed = field.config.indexed == true
        or field.config.alias ~= nil
        or ({
            select = true,
            boolean = true,
            date = true,
            datetime = true,
            number = true,
            currency = true,
        })[field.data_type] == true
    common.searchable = SEARCHABLE[field.data_type] == true and field.config.searchable ~= false
    return common
end

local function compile_object(crm_id: string, source: ObjectSource, attributes: { FieldSource }, dialect: string): CompiledObject
    local authored = clone(source)
    authored.object_id = nil
    authored.explicit_id = nil
    local object: CompiledObject = {
        object_id = source.object_id,
        explicit_id = source.explicit_id,
        object_type = source.type,
        label = source.label,
        icon = source.icon,
        plural = source.plural,
        is_syncable = source.is_syncable == true,
        definition = stable(authored),
        table_name = identifier("spiralscout_crm_obj_", crm_id .. "\0" .. source.object_id),
        fields = {},
        relations = {},
    }
    for _, field in ipairs(attributes) do
        if field.object_type == source.type then
            local compiled = compile_field(crm_id, source, field, dialect)
            if compiled.data_type == "relation" then
                object.relations[#object.relations + 1] = compiled
            else
                object.fields[#object.fields + 1] = compiled
            end
        end
    end
    return object
end

function M.compile(dialect: string, crm_id: string, declaration: unknown): Plan
    if dialect ~= "sqlite" and dialect ~= "postgres" then
        error("unsupported dialect: " .. tostring(dialect))
    end
    if trim(crm_id) == "" then
        error("crm_id is required")
    end
    local normalized = normalize(declaration)
    local manifest = stable(normalized)
    local source_hash = tostring(hash.sha256(manifest))
    local plan: Plan = {
        crm_id = crm_id,
        dialect = dialect,
        -- revision is the exact applied declaration token used for optimistic writes.
        -- Authored version is metadata and may remain stable across many UI edits.
        revision = source_hash,
        source_hash = source_hash,
        manifest = manifest,
        declaration = normalized,
        objects = {},
    }
    for _, object in ipairs(normalized.objects) do
        plan.objects[#plan.objects + 1] = compile_object(crm_id, object, normalized.attributes, dialect)
    end
    return plan
end

-- The static global catalog tables (fixed names, shared across every component) are
-- declared as tracked migration DDL, not built here. This module owns only the
-- dynamic per-component physical tables (spiralscout_crm_obj_*, _rel_*, _fts_*) that
-- schema provisioning materializes from a user-defined CRM object schema.

local function scalar_columns(field: CompiledField, _dialect: string): { string }
    return { field.column_name .. " " .. field.sql_type }
end

local function create_relation(db: Executor, relation: CompiledField, d: string)
    local columns = {
        "crm_id TEXT NOT NULL",
        "source_id TEXT NOT NULL",
        "ordinal INTEGER NOT NULL",
        "target_kind TEXT NOT NULL",
        "target_type TEXT NOT NULL",
        "target_id TEXT NOT NULL",
        "created_at " .. time_type(d) .. " NOT NULL",
        "updated_at " .. time_type(d) .. " NOT NULL",
        "PRIMARY KEY (crm_id, source_id, ordinal)",
        "UNIQUE (crm_id, source_id, target_kind, target_type, target_id)",
    }
    if relation.cardinality == "one" then
        columns[#columns + 1] = "CHECK (ordinal = 1)"
    end
    exec(db, "CREATE TABLE " .. relation.relation_table .. " (" .. table.concat(columns, ", ") .. ")")
    exec(
        db,
        "CREATE INDEX "
            .. identifier("idx_crm_", relation.relation_table .. "_target")
            .. " ON "
            .. relation.relation_table
            .. " (crm_id, target_kind, target_type, target_id)"
    )
end

local function create_object(db: Executor, object: CompiledObject, d: string)
    local columns = {
        "crm_id TEXT NOT NULL",
        "record_id TEXT NOT NULL",
        "canonical_id TEXT",
        "superseded_at " .. time_type(d),
        "created_at " .. time_type(d) .. " NOT NULL",
        "updated_at " .. time_type(d) .. " NOT NULL",
    }
    for _, field in ipairs(object.fields) do
        for _, column in ipairs(scalar_columns(field, d)) do
            columns[#columns + 1] = column
        end
    end
    columns[#columns + 1] = "PRIMARY KEY (crm_id, record_id)"
    exec(db, "CREATE TABLE " .. object.table_name .. " (" .. table.concat(columns, ", ") .. ")")
    exec(
        db,
        "CREATE INDEX "
            .. identifier("idx_crm_", object.table_name .. "_canonical")
            .. " ON "
            .. object.table_name
            .. " (crm_id, canonical_id)"
    )
    for _, relation in ipairs(object.relations) do
        create_relation(db, relation, d)
    end
end

local function catalog_field(db: Executor, plan: Plan, field: CompiledField)
    local params, values = {}, {}
    for _, value in ipairs({ plan.crm_id, field.object_id, field.field_id, field.object_type, field.attr, field.data_type }) do
        values[#values + 1] = bind(params, value)
    end
    values[#values + 1] = bind(params, field.column_name)
    values[#values + 1] = bind(params, field.relation_table)
    values[#values + 1] = bind(params, field.target_kind)
    values[#values + 1] = bind(params, field.target_type)
    values[#values + 1] = bind(params, field.target_object_id)
    values[#values + 1] = bind(params, field.cardinality)
    values[#values + 1] = bind(params, field.indexed)
    values[#values + 1] = bind(params, field.searchable)
    values[#values + 1] = bind(params, field.label)
    values[#values + 1] = bind(params, stable(field.config))
    values[#values + 1] = bind(params, field.sort_order)
    values[#values + 1] = bind(params, field.system)
    values[#values + 1] = bind(params, field.definition)
    values[#values + 1] = bind(params, plan.revision)
    exec(
        db,
        [[INSERT INTO spiralscout_crm_schema_attribute (crm_id,object_id,field_id,object_type,attr,data_type,column_name,relation_table,target_kind,target_type,target_object_id,cardinality,indexed,searchable,label,config,sort_order,system,definition,revision) VALUES (]]
            .. table.concat(values, ",")
            .. [[)]],
        params
    )
end

local function insert_object_catalog(db: Executor, plan: Plan, object: CompiledObject)
    local params, values = {}, {}
    values[#values + 1] = bind(params, plan.crm_id)
    values[#values + 1] = bind(params, object.object_id)
    values[#values + 1] = bind(params, object.object_type)
    values[#values + 1] = bind(params, object.table_name)
    values[#values + 1] = "NULL"
    values[#values + 1] = bind(params, object.label)
    values[#values + 1] = bind(params, object.icon)
    values[#values + 1] = bind(params, object.plural)
    values[#values + 1] = bind(params, object.is_syncable)
    values[#values + 1] = bind(params, object.definition)
    values[#values + 1] = bind(params, plan.revision)
    exec(
        db,
        [[INSERT INTO spiralscout_crm_schema_object (crm_id,object_id,object_type,table_name,fts_table,label,icon,plural,is_syncable,definition,revision) VALUES (]]
            .. table.concat(values, ",")
            .. [[)]],
        params
    )
end

local function update_object_catalog(db: Executor, plan: Plan, object: CompiledObject)
    local params = {}
    local object_type = bind(params, object.object_type)
    local label = bind(params, object.label)
    local icon = bind(params, object.icon)
    local plural = bind(params, object.plural)
    local syncable = bind(params, object.is_syncable)
    local definition = bind(params, object.definition)
    local revision = bind(params, plan.revision)
    local crm_id = bind(params, plan.crm_id)
    local object_id = bind(params, object.object_id)
    exec(
        db,
        "UPDATE spiralscout_crm_schema_object SET object_type="
            .. object_type
            .. ",label="
            .. label
            .. ",icon="
            .. icon
            .. ",plural="
            .. plural
            .. ",is_syncable="
            .. syncable
            .. ",definition="
            .. definition
            .. ",revision="
            .. revision
            .. " WHERE crm_id="
            .. crm_id
            .. " AND object_id="
            .. object_id,
        params
    )
end
local function rebuild_search(db: Executor, d: string, object: CompiledObject, fields: { CompiledField })
    local searchable = {}
    for _, field in ipairs(fields) do
        if field.searchable then
            searchable[#searchable + 1] = field
        end
    end
    local index = identifier("idx_crm_", object.table_name .. "_fts")
    if d == "postgres" then
        exec(db, "DROP INDEX IF EXISTS " .. index)
        exec(db, "ALTER TABLE " .. object.table_name .. " DROP COLUMN IF EXISTS search_document")
        if #searchable > 0 then
            local parts = {}
            for _, field in ipairs(searchable) do
                parts[#parts + 1] = "COALESCE(" .. ident(field.column_name) .. ", '')"
            end
            exec(
                db,
                "ALTER TABLE "
                    .. object.table_name
                    .. " ADD COLUMN search_document TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', "
                    .. table.concat(parts, " || ' ' || ")
                    .. ")) STORED"
            )
            exec(db, "CREATE INDEX " .. index .. " ON " .. object.table_name .. " USING GIN (search_document)")
        end
        object.fts_table = nil
    else
        local fts = identifier("spiralscout_crm_fts_", object.crm_id .. "\0" .. object.object_id)
        if object.fts_table then
            exec(db, "DROP TABLE IF EXISTS " .. ident(object.fts_table))
        end
        exec(db, "DROP TABLE IF EXISTS " .. fts)
        if #searchable > 0 then
            local cols = { "crm_id UNINDEXED", "record_id UNINDEXED" }
            for _, field in ipairs(searchable) do
                cols[#cols + 1] = ident(field.column_name)
            end
            exec(
                db,
                "CREATE VIRTUAL TABLE "
                    .. fts
                    .. " USING fts5("
                    .. table.concat(cols, ", ")
                    .. ", tokenize = 'unicode61 remove_diacritics 2')"
            )
            local names = { "crm_id", "record_id" }
            for _, field in ipairs(searchable) do
                names[#names + 1] = ident(field.column_name)
            end
            exec(
                db,
                "INSERT INTO "
                    .. fts
                    .. " ("
                    .. table.concat(names, ",")
                    .. ") SELECT "
                    .. table.concat(names, ",")
                    .. " FROM "
                    .. object.table_name
            )
            object.fts_table = fts
        else
            object.fts_table = nil
        end
    end
    local params = {}
    local fts = bind(params, object.fts_table)
    local crm = bind(params, object.crm_id)
    local object_id = bind(params, object.object_id)
    exec(
        db,
        "UPDATE spiralscout_crm_schema_object SET fts_table="
            .. fts
            .. " WHERE crm_id="
            .. crm
            .. " AND object_id="
            .. object_id,
        params
    )
end

local function current_objects(db: Executor, crm_id: string): { [string]: CompiledObject }
    local out: { [string]: CompiledObject } = {}
    for _, row in ipairs(rows(db, "SELECT * FROM spiralscout_crm_schema_object WHERE crm_id=$1", { crm_id })) do
        row.table_name = ident(row.table_name)
        if row.fts_table then
            row.fts_table = ident(row.fts_table)
        end
        out[tostring(row.object_id)] = row :: CompiledObject
    end
    return out
end

local function current_fields(db: Executor, crm_id: string, object_id: string): { [string]: CompiledField }
    local out: { [string]: CompiledField } = {}
    for _, row in ipairs(rows(
        db,
        "SELECT * FROM spiralscout_crm_schema_attribute WHERE crm_id=$1 AND object_id=$2",
        { crm_id, object_id }
    )) do
        if row.column_name then
            row.column_name = ident(row.column_name)
        else
            row.relation_table = ident(row.relation_table)
        end
        out[tostring(row.field_id)] = row :: CompiledField
    end
    return out
end

local function mark(db: Executor, plan: Plan, status: string, message: unknown, applied: unknown)
    local params, values = {}, {}
    for _, value in ipairs({ plan.crm_id, plan.revision, plan.source_hash, plan.dialect, status }) do
        values[#values + 1] = bind(params, value)
    end
    values[#values + 1] = bind(params, message)
    values[#values + 1] = bind(params, os.date("!%Y-%m-%dT%H:%M:%SZ"))
    values[#values + 1] = bind(params, applied)
    exec(
        db,
        [[INSERT INTO spiralscout_crm_schema_apply (crm_id,revision,source_hash,dialect,status,error,started_at,finished_at) VALUES (]]
            .. table.concat(values, ",")
            .. [[) ON CONFLICT (crm_id) DO UPDATE SET revision=excluded.revision,source_hash=excluded.source_hash,dialect=excluded.dialect,status=excluded.status,error=excluded.error,started_at=excluded.started_at,finished_at=excluded.finished_at]],
        params
    )
end

local function update_index(db: Executor, dialect: string, object: CompiledObject, field: CompiledField, wanted: boolean)
    local name = identifier("idx_crm_", object.table_name .. "_" .. field.field_id)
    local unique_name = identifier("uq_crm_", object.table_name .. "_" .. field.field_id)
    local unique = type(field.config) == "table" and field.config.unique == true
    if unique then
        local duplicate = rows(
            db,
            "SELECT " .. field.column_name .. " FROM " .. object.table_name
                .. " WHERE " .. field.column_name .. " IS NOT NULL GROUP BY " .. field.column_name
                .. " HAVING COUNT(*) > 1 LIMIT 1"
        )[1]
        if duplicate then
            error("CRM unique field conflict: " .. tostring(field.object_type) .. "." .. tostring(field.attr), 0)
        end
        exec(db, "DROP INDEX IF EXISTS " .. name)
        local ok = pcall(function()
            local indexed_value = field.column_name
            if dialect == "postgres" and field.data_type == "multi_select" then
                indexed_value = "(" .. field.column_name .. "::text)"
            end
            exec(
                db,
                "CREATE UNIQUE INDEX IF NOT EXISTS " .. unique_name .. " ON " .. object.table_name
                    .. " (crm_id, " .. indexed_value .. ") WHERE " .. field.column_name .. " IS NOT NULL"
            )
        end)
        if not ok then
            error("CRM unique field conflict: " .. tostring(field.object_type) .. "." .. tostring(field.attr), 0)
        end
    elseif wanted then
        exec(db, "DROP INDEX IF EXISTS " .. unique_name)
        local using = dialect == "postgres" and field.data_type == "multi_select" and " USING GIN" or ""
        exec(
            db,
            "CREATE INDEX IF NOT EXISTS "
                .. name
                .. " ON "
                .. object.table_name
                .. using
                .. " ("
                .. field.column_name
                .. ")"
        )
    else
        exec(db, "DROP INDEX IF EXISTS " .. name)
        exec(db, "DROP INDEX IF EXISTS " .. unique_name)
    end
end

function M.unique_index_name(object: unknown, field: unknown): string
    if type(object) ~= "table" or type((object :: { table_name: unknown? }).table_name) ~= "string" then
        error("compiled CRM object is required")
    end
    if type(field) ~= "table" or type((field :: { field_id: unknown? }).field_id) ~= "string" then
        error("compiled CRM field is required")
    end
    local table_name = (object :: { table_name: string }).table_name
    local field_id = (field :: { field_id: string }).field_id
    return identifier("uq_crm_", table_name .. "_" .. field_id)
end

local function upsert_declarations(db: Executor, plan: Plan)
    local defs = {
        { "config", "root", { version = plan.declaration.version, merge_config = plan.declaration.merge_config } },
    }
    for _, pipeline in ipairs(plan.declaration.pipelines) do
        defs[#defs + 1] = { "pipeline", trim(pipeline.collection_id), pipeline }
    end
    for _, view in ipairs(plan.declaration.views) do
        defs[#defs + 1] = { "view", trim(view.view_id), view }
    end
    for _, policy in ipairs(plan.declaration.policies) do
        defs[#defs + 1] = {
            "policy",
            trim(policy.portableKey) ~= "" and trim(policy.portableKey) or trim(policy.key),
            policy,
        }
    end
    for _, definition in ipairs(defs) do
        if definition[2] == "" then
            error(definition[1] .. " declaration id is required")
        end
        exec(
            db,
            [[INSERT INTO spiralscout_crm_schema_declaration (crm_id,kind,declaration_id,definition,revision) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (crm_id,kind,declaration_id) DO UPDATE SET definition=excluded.definition,revision=excluded.revision]],
            { plan.crm_id, definition[1], definition[2], stable(definition[3]), plan.revision }
        )
    end
end
function M.apply(db: sql.DB, dialect: string, crm_id: string, declaration: unknown, expected_revision: unknown): ApplyResult
    local plan: Plan = M.compile(dialect, crm_id, declaration)
    local tx, begin_err = db:begin()
    if begin_err then
        mark(db, plan, "failed", tostring(begin_err), nil)
        error(begin_err)
    end
    local unchanged = false
    local ok, err = pcall(function()
        local lock = dialect == "postgres" and " FOR UPDATE" or ""
        local prior = rows(tx, "SELECT revision,source_hash FROM spiralscout_crm_schema_revision WHERE crm_id=$1" .. lock,
            { crm_id })[1]
        if prior and prior.source_hash == plan.source_hash then unchanged = true; return end
        if expected_revision ~= nil and (not prior or tostring(prior.revision) ~= tostring(expected_revision)) then
            error("CRM schema revision conflict", 0)
        end
        mark(tx, plan, "applying", nil, nil)
        local existing = current_objects(tx, crm_id)
        for _, raw_object in ipairs(plan.objects) do
            local object = raw_object :: CompiledObject
            object.crm_id = crm_id
            local old = existing[object.object_id]
            if not old then
                create_object(tx, object, dialect)
                insert_object_catalog(tx, plan, object)
                for _, raw_field in ipairs(object.fields) do
                    local field = raw_field :: CompiledField
                    catalog_field(tx, plan, field)
                    update_index(tx, dialect, object, field, field.indexed)
                end
                for _, raw_field in ipairs(object.relations) do
                    local field = raw_field :: CompiledField
                    catalog_field(tx, plan, field)
                end
            else
                object.table_name = old.table_name
                object.fts_table = old.fts_table
                if old.object_type ~= object.object_type and not object.explicit_id then
                    error("object rename requires explicit stable id: " .. old.object_type)
                end
                if old.object_type ~= object.object_type then
                    exec(
                        tx,
                        "UPDATE spiralscout_crm_schema_attribute SET object_type=$1 WHERE crm_id=$2 AND object_id=$3",
                        { object.object_type, crm_id, object.object_id }
                    )
                    exec(
                        tx,
                        "UPDATE spiralscout_crm_record_locator SET object_type=$1 WHERE crm_id=$2 AND object_type=$3",
                        { object.object_type, crm_id, old.object_type }
                    )
                    exec(
                        tx,
                        "UPDATE spiralscout_crm_alias SET object_scope=$1 WHERE crm_id=$2 AND object_scope=$3",
                        { object.object_type, crm_id, old.object_type }
                    )
                    exec(
                        tx,
                        "UPDATE spiralscout_crm_alias_reservation SET object_scope=$1 WHERE crm_id=$2 AND object_scope=$3",
                        { object.object_type, crm_id, old.object_type }
                    )
                    exec(
                        tx,
                        "UPDATE spiralscout_crm_merge_decision SET object_type=$1 WHERE crm_id=$2 AND object_type=$3",
                        { object.object_type, crm_id, old.object_type }
                    )
                    exec(
                        tx,
                        "UPDATE spiralscout_crm_write_origin SET object_type=$1 WHERE crm_id=$2 AND object_type=$3",
                        { object.object_type, crm_id, old.object_type }
                    )
                end
                update_object_catalog(tx, plan, object)
                local fields = current_fields(tx, crm_id, object.object_id)
                local desired: { CompiledField } = {}
                for _, field in ipairs(object.fields) do
                    desired[#desired + 1] = field
                end
                for _, field in ipairs(object.relations) do
                    desired[#desired + 1] = field
                end
                for _, raw_field in ipairs(desired) do
                    local field = raw_field :: CompiledField
                    local prior_field = fields[field.field_id]
                    if prior_field then
                        if prior_field.attr ~= field.attr and not field.explicit_id then
                            error("attribute rename requires explicit stable id: " .. prior_field.attr)
                        end
                        if prior_field.data_type ~= field.data_type then
                            error(
                                "field type change requires explicit data migration: "
                                    .. field.object_type
                                    .. "."
                                    .. field.attr
                            )
                        end
                        local target_changed = field.data_type == "relation"
                            and (
                                prior_field.target_kind ~= field.target_kind
                                or prior_field.cardinality ~= field.cardinality
                            )
                        if field.data_type == "relation" and field.target_kind == "record" then
                            target_changed = target_changed or prior_field.target_object_id ~= field.target_object_id
                        elseif field.data_type == "relation" then
                            target_changed = target_changed or prior_field.target_type ~= field.target_type
                        end
                        if target_changed then
                            error(
                                "relation contract change requires explicit data migration: "
                                    .. field.object_type
                                    .. "."
                                    .. field.attr
                            )
                        end
                        field.column_name = prior_field.column_name
                        field.relation_table = prior_field.relation_table
                        if field.column_name then
                            field.column_name = ident(field.column_name)
                        else
                            field.relation_table = ident(field.relation_table)
                        end
                        if
                            field.relation_table
                            and field.target_kind == "record"
                            and prior_field.target_type ~= field.target_type
                        then
                            exec(
                                tx,
                                "UPDATE "
                                    .. field.relation_table
                                    .. " SET target_type=$1,updated_at=$2 WHERE crm_id=$3 AND target_kind='record' AND target_type=$4",
                                {
                                    field.target_type,
                                    os.date("!%Y-%m-%dT%H:%M:%SZ"),
                                    crm_id,
                                    prior_field.target_type,
                                }
                            )
                        end
                        exec(
                            tx,
                            "DELETE FROM spiralscout_crm_schema_attribute WHERE crm_id=$1 AND object_id=$2 AND field_id=$3",
                            { crm_id, object.object_id, field.field_id }
                        )
                        catalog_field(tx, plan, field)
                        if field.column_name then
                            update_index(tx, dialect, object, field, field.indexed)
                        end
                    else
                        if field.column_name then
                            for _, column in ipairs(scalar_columns(field, dialect)) do
                                exec(tx, "ALTER TABLE " .. object.table_name .. " ADD COLUMN " .. column)
                            end
                            catalog_field(tx, plan, field)
                            update_index(tx, dialect, object, field, field.indexed)
                        else
                            create_relation(tx, field, dialect)
                            catalog_field(tx, plan, field)
                        end
                    end
                end
            end
            local final: { CompiledField } = {}
            for _, field in pairs(current_fields(tx, crm_id, object.object_id)) do
                if field.column_name then
                    final[#final + 1] = field
                end
            end
            rebuild_search(tx, dialect, object, final)
        end
        -- Missing declarations are retained. Destruction requires explicit remove lists.
        local remove = plan.declaration.remove
        local removed_objects = {}
        for _, item in ipairs(type(remove.objects) == "table" and remove.objects or {}) do
            local object_id = trim(type(item) == "table" and item.object_id or item)
            if object_id == "" then error("explicit object removal requires object_id") end
            if removed_objects[object_id] then error("duplicate object removal: " .. object_id) end
            removed_objects[object_id] = true
        end
        -- Views and pipelines are declaration rows and nothing else -- no
        -- tables, no columns -- so their removal is the row's. Same law as the
        -- rest of remove: destruction only by naming, never by omission, and a
        -- name that resolves to nothing is an error rather than a silent no-op.
        local function remove_declaration(kind: string, declaration_id: string)
            local existing = rows(
                tx,
                "SELECT declaration_id FROM spiralscout_crm_schema_declaration WHERE crm_id=$1 AND kind=$2 AND declaration_id=$3",
                { crm_id, kind, declaration_id }
            )
            if #existing == 0 then
                error(kind .. " removal target not found: " .. declaration_id)
            end
            exec(
                tx,
                "DELETE FROM spiralscout_crm_schema_declaration WHERE crm_id=$1 AND kind=$2 AND declaration_id=$3",
                { crm_id, kind, declaration_id }
            )
        end
        for _, item in ipairs(type(remove.views) == "table" and remove.views or {}) do
            local view_id = trim(type(item) == "table" and item.view_id or item)
            if view_id == "" then error("explicit view removal requires view_id") end
            remove_declaration("view", view_id)
        end
        for _, item in ipairs(type(remove.pipelines) == "table" and remove.pipelines or {}) do
            local collection_id = trim(type(item) == "table" and item.collection_id or item)
            if collection_id == "" then error("explicit pipeline removal requires collection_id") end
            remove_declaration("pipeline", collection_id)
        end
        for _, item in ipairs(type(remove.attributes) == "table" and remove.attributes or {}) do
            local object_id = trim(item.object_id)
            local field_id = trim(item.field_id)
            if object_id == "" or field_id == "" then
                error("explicit attribute removal requires object_id and field_id")
            end
            local object = current_objects(tx, crm_id)[object_id] :: CompiledObject
            local field = object and current_fields(tx, crm_id, object_id)[field_id] or nil
            if not field then
                error("attribute removal target not found")
            end
            object.crm_id = crm_id
            if field.relation_table then
                exec(tx, "DROP TABLE " .. ident(field.relation_table))
            else
                rebuild_search(tx, dialect, object, {})
                local index = identifier("idx_crm_", object.table_name .. "_" .. field_id)
                exec(tx, "DROP INDEX IF EXISTS " .. index)
                exec(tx, "DROP INDEX IF EXISTS " .. identifier("uq_crm_", object.table_name .. "_" .. field_id))
                exec(tx, "ALTER TABLE " .. object.table_name .. " DROP COLUMN " .. ident(field.column_name))
            end
            exec(
                tx,
                "DELETE FROM spiralscout_crm_schema_attribute WHERE crm_id=$1 AND object_id=$2 AND field_id=$3",
                { crm_id, object_id, field_id }
            )
            local remaining: { CompiledField } = {}
            for _, remaining_field in pairs(current_fields(tx, crm_id, object_id)) do
                if remaining_field.column_name then
                    remaining[#remaining + 1] = remaining_field
                end
            end
            rebuild_search(tx, dialect, object, remaining)
        end
        local incoming = {}
        for source_id, source in pairs(current_objects(tx, crm_id)) do
            if not removed_objects[source_id] then
                for _, field in pairs(current_fields(tx, crm_id, source_id)) do
                    if field.relation_table and field.target_kind == "record" and removed_objects[field.target_object_id] then
                        incoming[#incoming + 1] = tostring(source.object_type) .. "." .. tostring(field.attr)
                    end
                end
            end
        end
        if #incoming > 0 then
            table.sort(incoming)
            error("cannot remove CRM object while retained relations target it: " .. table.concat(incoming, ", "))
        end
        for _, item in ipairs(type(remove.objects) == "table" and remove.objects or {}) do
            local object_id = trim(type(item) == "table" and item.object_id or item)
            if object_id == "" then
                error("explicit object removal requires object_id")
            end
            local object = current_objects(tx, crm_id)[object_id]
            if not object then
                error("object removal target not found")
            end
            for _, field in pairs(current_fields(tx, crm_id, object_id)) do
                if field.relation_table then
                    exec(tx, "DROP TABLE " .. ident(field.relation_table))
                end
            end
            local ids = "SELECT record_id FROM spiralscout_crm_record_locator WHERE crm_id=$1 AND object_type=$2"
            exec(
                tx,
                "DELETE FROM spiralscout_crm_collection_membership WHERE crm_id=$1 AND record_id IN (" .. ids .. ")",
                { crm_id, object.object_type }
            )
            exec(
                tx,
                "DELETE FROM spiralscout_crm_activity WHERE crm_id=$1 AND (record_id IN ("
                    .. ids
                    .. ") OR participant_record_id IN ("
                    .. ids
                    .. "))",
                { crm_id, object.object_type }
            )
            exec(
                tx,
                "DELETE FROM spiralscout_crm_alias WHERE crm_id=$1 AND object_scope=$2",
                { crm_id, object.object_type }
            )
            exec(
                tx,
                "DELETE FROM spiralscout_crm_alias_reservation WHERE crm_id=$1 AND object_scope=$2",
                { crm_id, object.object_type }
            )
            exec(
                tx,
                "DELETE FROM spiralscout_crm_merge_decision WHERE crm_id=$1 AND object_type=$2",
                { crm_id, object.object_type }
            )
            exec(
                tx,
                "DELETE FROM spiralscout_crm_record_locator WHERE crm_id=$1 AND object_type=$2",
                { crm_id, object.object_type }
            )
            if object.fts_table then
                exec(tx, "DROP TABLE IF EXISTS " .. ident(object.fts_table))
            end
            exec(tx, "DROP TABLE " .. ident(object.table_name))
            exec(
                tx,
                "DELETE FROM spiralscout_crm_schema_attribute WHERE crm_id=$1 AND object_id=$2",
                { crm_id, object_id }
            )
            exec(
                tx,
                "DELETE FROM spiralscout_crm_schema_object WHERE crm_id=$1 AND object_id=$2",
                { crm_id, object_id }
            )
        end
        upsert_declarations(tx, plan)
        for _, item in ipairs(type(remove.declarations) == "table" and remove.declarations or {}) do
            local kind = trim(type(item) == "table" and item.kind or nil)
            local id = trim(type(item) == "table" and item.id or nil)
            if (kind ~= "pipeline" and kind ~= "view" and kind ~= "policy") or id == "" then
                error("explicit declaration removal requires kind and id")
            end
            exec(
                tx,
                "DELETE FROM spiralscout_crm_schema_declaration WHERE crm_id=$1 AND kind=$2 AND declaration_id=$3",
                { crm_id, kind, id }
            )
        end
        local applied_at = os.date("!%Y-%m-%dT%H:%M:%SZ")
        exec(
            tx,
            [[INSERT INTO spiralscout_crm_schema_revision (crm_id,revision,source_hash,dialect,applied_at) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (crm_id) DO UPDATE SET revision=excluded.revision,source_hash=excluded.source_hash,dialect=excluded.dialect,applied_at=excluded.applied_at]],
            { crm_id, plan.revision, plan.source_hash, dialect, applied_at }
        )
        mark(tx, plan, "applied", nil, applied_at)
    end)
    if not ok then
        tx:rollback()
        mark(db, plan, "failed", tostring(err), nil)
        error(err, 0)
    end
    local _, commit_err = tx:commit()
    if commit_err then
        tx:rollback()
        mark(db, plan, "failed", tostring(commit_err), nil)
        error(commit_err, 0)
    end
    return {
        changed = not unchanged,
        revision = plan.revision,
        source_hash = plan.source_hash,
        objects = plan.objects,
    }
end

-- Drops the dynamic per-component tables the runtime materialized (object, relation,
-- and FTS tables named from the schema catalog). Their names are only knowable from
-- spiralscout_crm_schema_object, so this teardown is dynamic and stays in the compiler;
-- the static catalog tables are dropped by the migration that declares them.
function M.drop_component_tables(db: Executor): boolean
    for _, object in ipairs(rows(db, "SELECT crm_id,object_id,table_name,fts_table FROM spiralscout_crm_schema_object")) do
        for _, field in ipairs(rows(
            db,
            "SELECT relation_table FROM spiralscout_crm_schema_attribute WHERE crm_id=$1 AND object_id=$2 AND relation_table IS NOT NULL",
            { object.crm_id, object.object_id }
        )) do
            exec(db, "DROP TABLE IF EXISTS " .. ident(field.relation_table))
        end
        if object.fts_table then
            exec(db, "DROP TABLE IF EXISTS " .. ident(object.fts_table))
        end
        exec(db, "DROP TABLE IF EXISTS " .. ident(object.table_name))
    end
    return true
end

M.identifier = identifier
M.stable_encode = stable
return M
