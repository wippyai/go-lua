-- SQL data-access for the PIM schema: channels, attribute groups, attributes,
-- attribute options, and families. Owns the versioned reconcile of the shipped
-- configuration (keyed by the append-only schema ledger) and the management
-- write path. Every write validates through attribute_config and stamps
-- created_by/updated_by provenance; disable is the only subtractive verb and
-- nothing here ever deletes product values.

local json = require("json")
local sql = require("sql")
local time = require("time")
local uuid = require("uuid")
local config = require("config")
local attribute_config = require("attribute_config")

local M = {}

local DB = "app:db"
type SqlParams = { unknown }
type SqlRow = { [string]: unknown }
type SqlRows = { SqlRow }
type Executor = sql.DB | sql.Transaction

local function now()
    return time.now():utc():format_rfc3339()
end

local function new_id()
    return uuid.v4()
end

local function encode(v)
    local out, err = json.encode(v == nil and {} or v)
    if err or not out then return "{}" end
    return out
end

local function decode(raw)
    if raw == nil or raw == "" then return nil end
    if type(raw) == "table" then return raw end
    local out, err = json.decode(tostring(raw))
    if err then return nil end
    return out
end

-- Cross-dialect boolean: Postgres yields true/false, SQLite yields 0/1.
local function as_bool(v)
    return v == true or v == 1 or v == "1" or v == "t" or v == "true"
end

local function open()
    local db, err = sql.get(DB)
    if err or not db then return nil, "pim database unavailable: " .. tostring(err) end
    return db, nil
end

local function query(h: Executor, q: string, params: SqlParams?): (SqlRows?, string?)
    local rows, err = h:query(q, params or {})
    if err then return nil, tostring(err) end
    return (rows or {}) :: SqlRows, nil
end

local function exec(h: Executor, q: string, params: SqlParams?): string?
    local _, err = h:execute(q, params or {})
    if err then return tostring(err) end
    return nil
end

local function errors_text(errors)
    local parts = {}
    for _, e in ipairs(errors or {}) do
        parts[#parts + 1] = tostring(e.path) .. ": " .. tostring(e.message)
    end
    return table.concat(parts, "; ")
end

-- ─── row mappers ─────────────────────────────────────────────────────────

function M.map_attribute(r)
    return {
        code = tostring(r.code or ""),
        type = r.type,
        attribute_group = r.attribute_group,
        labels = decode(r.labels) or {},
        config = decode(r.config) or {},
        localizable = as_bool(r.localizable),
        scopable = as_bool(r.scopable),
        unique_value = as_bool(r.unique_value),
        searchable = as_bool(r.searchable),
        filterable = as_bool(r.filterable),
        enabled = as_bool(r.enabled),
        system = as_bool(r.system),
        sort_order = tonumber(r.sort_order) or 0,
        created_by = r.created_by,
        updated_by = r.updated_by,
        updated_at = r.updated_at,
    }
end

local function map_option(r)
    return {
        code = tostring(r.code or ""),
        labels = decode(r.labels) or {},
        aliases = decode(r.aliases) or {},
        enabled = as_bool(r.enabled),
        system = as_bool(r.system),
        sort_order = tonumber(r.sort_order) or 0,
        created_by = r.created_by,
        updated_by = r.updated_by,
    }
end

local function map_channel(r)
    return {
        code = tostring(r.code or ""),
        labels = decode(r.labels) or {},
        locales = decode(r.locales) or {},
        currencies = decode(r.currencies) or {},
        enabled = as_bool(r.enabled),
        system = as_bool(r.system),
        created_by = r.created_by,
        updated_by = r.updated_by,
    }
end

local function map_group(r)
    return {
        code = tostring(r.code or ""),
        labels = decode(r.labels) or {},
        sort_order = tonumber(r.sort_order) or 0,
        system = as_bool(r.system),
        created_by = r.created_by,
        updated_by = r.updated_by,
    }
end

-- ─── declared attribute shape <-> storage split ──────────────────────────

-- Top-level attribute columns; every other declared key lands in the config
-- blob attribute_config reads back. Options travel through their own storage.
local ATTR_TOP = {
    code = true, type = true, group = true, labels = true, unique = true,
    searchable = true, filterable = true, localizable = true, scopable = true,
    sort_order = true, system = true, enabled = true, options = true,
}

local function split_config(def)
    local cfg = {}
    for k, v in pairs(def) do
        if not ATTR_TOP[k] then cfg[k] = v end
    end
    return cfg
end

-- Mapped row back to the declared attribute shape validate_attribute /
-- validate_value expect (type config lifted from the config blob to top level).
function M.to_definition(attr)
    local def = {}
    for k, v in pairs(attr.config or {}) do def[k] = v end
    def.code = attr.code
    def.type = attr.type
    def.group = attr.attribute_group
    def.labels = attr.labels
    def.unique = attr.unique_value
    def.searchable = attr.searchable
    def.filterable = attr.filterable
    def.localizable = attr.localizable
    def.scopable = attr.scopable
    def.sort_order = attr.sort_order
    def.enabled = attr.enabled
    return def
end

local function insert_attribute(h, def, system, actor_id, ts)
    return exec(h, [[
        INSERT INTO spiralscout_pim_attributes
            (code, type, attribute_group, labels, config, localizable, scopable, unique_value, searchable, filterable,
             enabled, system, sort_order, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
    ]], {
        def.code, def.type, def.group, encode(def.labels or {}), encode(split_config(def)),
        def.localizable == true, def.scopable == true, def.unique == true,
        def.searchable == true, def.filterable == true, def.enabled ~= false,
        system == true, tonumber(def.sort_order) or 0, actor_id, actor_id, ts, ts,
    })
end

-- ─── reconcile ───────────────────────────────────────────────────────────

local function arr(v)
    return type(v) == "table" and v or {}
end

local function cfg_version(cfg)
    local v = type(cfg) == "table" and cfg.version or nil
    if type(v) == "string" and v ~= "" then return v end
    if type(v) == "number" then return tostring(v) end
    return "1"
end

local function ledger_keys(h)
    local rows, err = query(h, "SELECT portable_key, version FROM spiralscout_pim_schema_ledger", {})
    if err then return nil, err end
    local out = {}
    for _, r in ipairs(rows) do
        out[tostring(r.portable_key) .. "\n" .. tostring(r.version)] = true
    end
    return out, nil
end

local function ledger_insert(h, key, version, ts)
    return exec(h, [[
        INSERT INTO spiralscout_pim_schema_ledger (id, portable_key, version, applied_at)
        VALUES ($1, $2, $3, $4)
    ]], { new_id(), key, version, ts })
end

local function apply_channel(h, ch, ts)
    local rows, err = query(h, "SELECT system FROM spiralscout_pim_channels WHERE code = $1", { ch.code })
    if err then return err end
    if not rows[1] then
        return exec(h, [[
            INSERT INTO spiralscout_pim_channels (code, labels, locales, currencies, enabled, system, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ]], { ch.code, encode(ch.labels or {}), encode(ch.locales or {}), encode(ch.currencies or {}), true, true, ts, ts })
    end
    if not as_bool(rows[1].system) then return nil end
    return exec(h, "UPDATE spiralscout_pim_channels SET labels = $1, updated_at = $2 WHERE code = $3",
        { encode(ch.labels or {}), ts, ch.code })
end

local function apply_group(h, g, ts)
    local rows, err = query(h, "SELECT system FROM spiralscout_pim_attribute_groups WHERE code = $1", { g.code })
    if err then return err end
    if not rows[1] then
        return exec(h, [[
            INSERT INTO spiralscout_pim_attribute_groups (code, labels, sort_order, system, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6)
        ]], { g.code, encode(g.labels or {}), tonumber(g.sort_order) or 0, true, ts, ts })
    end
    if not as_bool(rows[1].system) then return nil end
    return exec(h, "UPDATE spiralscout_pim_attribute_groups SET labels = $1, sort_order = $2, updated_at = $3 WHERE code = $4",
        { encode(g.labels or {}), tonumber(g.sort_order) or 0, ts, g.code })
end

-- Existing system rows take only presentation fields (labels, group,
-- sort_order); type, flags, config, enabled, and user rows stay untouched.
local function apply_attribute(h, attr, ts)
    local rows, err = query(h, "SELECT system FROM spiralscout_pim_attributes WHERE code = $1", { attr.code })
    if err then return err end
    if not rows[1] then
        return insert_attribute(h, attr, true, nil, ts)
    end
    if not as_bool(rows[1].system) then return nil end
    if attr.group == nil then
        return exec(h, [[
            UPDATE spiralscout_pim_attributes SET labels = $1, attribute_group = NULL, sort_order = $2, updated_at = $3 WHERE code = $4
        ]], { encode(attr.labels or {}), tonumber(attr.sort_order) or 0, ts, attr.code })
    end
    return exec(h, [[
        UPDATE spiralscout_pim_attributes SET labels = $1, attribute_group = $2, sort_order = $3, updated_at = $4 WHERE code = $5
    ]], { encode(attr.labels or {}), attr.group, tonumber(attr.sort_order) or 0, ts, attr.code })
end

local function apply_option(h, attr_code, opt, ts)
    local arows, aerr = query(h, "SELECT system FROM spiralscout_pim_attributes WHERE code = $1", { attr_code })
    if aerr then return aerr end
    if not arows[1] or not as_bool(arows[1].system) then return nil end
    local rows, err = query(h, "SELECT system FROM spiralscout_pim_attribute_options WHERE attribute_code = $1 AND code = $2",
        { attr_code, opt.code })
    if err then return err end
    if not rows[1] then
        return exec(h, [[
            INSERT INTO spiralscout_pim_attribute_options (option_id, attribute_code, code, labels, aliases, enabled, system, sort_order, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        ]], { new_id(), attr_code, opt.code, encode(opt.labels or {}), encode(opt.aliases or {}), true, true, tonumber(opt.sort_order) or 0, ts, ts })
    end
    if not as_bool(rows[1].system) then return nil end
    return exec(h, [[
        UPDATE spiralscout_pim_attribute_options SET labels = $1, sort_order = $2, updated_at = $3 WHERE attribute_code = $4 AND code = $5
    ]], { encode(opt.labels or {}), tonumber(opt.sort_order) or 0, ts, attr_code, opt.code })
end

local function insert_family_rows(h, fam, ts)
    for i, code in ipairs(arr(fam.attributes)) do
        local err = exec(h, "INSERT INTO spiralscout_pim_family_attributes (family_code, attribute_code, sort_order) VALUES ($1, $2, $3)",
            { fam.code, code, i * 10 })
        if err then return err end
    end
    for _, req in ipairs(arr(fam.requirements)) do
        local err = exec(h, [[
            INSERT INTO spiralscout_pim_family_requirements (requirement_id, family_code, channel_code, locale_code, required_attributes, created_at)
            VALUES ($1, $2, $3, $4, $5, $6)
        ]], { new_id(), fam.code, req.channel, req.locale, encode(req.required_attributes or {}), ts })
        if err then return err end
    end
    return nil
end

-- New families land whole; existing system families take labels plus additive
-- membership and requirement rows only.
local function apply_family(h, fam, ts)
    local rows, err = query(h, "SELECT system FROM spiralscout_pim_families WHERE code = $1", { fam.code })
    if err then return err end
    if not rows[1] then
        local ierr = exec(h, [[
            INSERT INTO spiralscout_pim_families (code, labels, attribute_as_label, attribute_as_image, config, system, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ]], { fam.code, encode(fam.labels or {}), fam.attribute_as_label, fam.attribute_as_image, "{}", true, ts, ts })
        if ierr then return ierr end
        return insert_family_rows(h, fam, ts)
    end
    if not as_bool(rows[1].system) then return nil end

    local uerr = exec(h, "UPDATE spiralscout_pim_families SET labels = $1, updated_at = $2 WHERE code = $3",
        { encode(fam.labels or {}), ts, fam.code })
    if uerr then return uerr end

    local existing = {}
    local mrows, merr = query(h, "SELECT attribute_code FROM spiralscout_pim_family_attributes WHERE family_code = $1", { fam.code })
    if merr then return merr end
    for _, r in ipairs(mrows) do existing[tostring(r.attribute_code)] = true end
    for i, code in ipairs(arr(fam.attributes)) do
        if not existing[code] then
            local ierr = exec(h, "INSERT INTO spiralscout_pim_family_attributes (family_code, attribute_code, sort_order) VALUES ($1, $2, $3)",
                { fam.code, code, i * 10 })
            if ierr then return ierr end
        end
    end

    local scopes = {}
    local rrows, rerr = query(h, "SELECT channel_code, locale_code FROM spiralscout_pim_family_requirements WHERE family_code = $1", { fam.code })
    if rerr then return rerr end
    for _, r in ipairs(rrows) do
        scopes[tostring(r.channel_code) .. "\n" .. tostring(r.locale_code or "")] = true
    end
    for _, req in ipairs(arr(fam.requirements)) do
        if not scopes[tostring(req.channel) .. "\n" .. tostring(req.locale or "")] then
            local ierr = exec(h, [[
                INSERT INTO spiralscout_pim_family_requirements (requirement_id, family_code, channel_code, locale_code, required_attributes, created_at)
                VALUES ($1, $2, $3, $4, $5, $6)
            ]], { new_id(), fam.code, req.channel, req.locale, encode(req.required_attributes or {}), ts })
            if ierr then return ierr end
        end
    end
    return nil
end

-- Ordered (portable_key, apply) pairs for every item the configuration declares.
local function config_items(cfg)
    local items = {}
    local function item(key, apply)
        items[#items + 1] = { key = key, apply = apply }
    end
    for _, ch in ipairs(arr(cfg.channels)) do
        if type(ch) == "table" and type(ch.code) == "string" then
            item("channel/" .. ch.code, function(h, ts) return apply_channel(h, ch, ts) end)
        end
    end
    for _, g in ipairs(arr(cfg.attribute_groups)) do
        if type(g) == "table" and type(g.code) == "string" then
            item("attribute_group/" .. g.code, function(h, ts) return apply_group(h, g, ts) end)
        end
    end
    for _, attr in ipairs(arr(cfg.attributes)) do
        if type(attr) == "table" and type(attr.code) == "string" then
            item("attribute/" .. attr.code, function(h, ts) return apply_attribute(h, attr, ts) end)
            for _, opt in ipairs(arr(attr.options)) do
                if type(opt) == "table" and type(opt.code) == "string" then
                    local attr_code = attr.code
                    item("option/" .. attr_code .. "/" .. opt.code, function(h, ts) return apply_option(h, attr_code, opt, ts) end)
                end
            end
        end
    end
    for _, fam in ipairs(arr(cfg.families)) do
        if type(fam) == "table" and type(fam.code) == "string" then
            item("family/" .. fam.code, function(h, ts) return apply_family(h, fam, ts) end)
        end
    end
    return items
end

-- Idempotent versioned reconcile: each (portable_key, version) pair applies at
-- most once, recorded in the ledger inside the same transaction. cfg defaults
-- to the effective configuration.
function M.reconcile(cfg)
    local effective = type(cfg) == "table" and cfg or config.default_config()
    local version = cfg_version(effective)
    local items = config_items(effective)
    if #items == 0 then return nil end

    local db, err = open()
    if not db then return err end
    local applied, lerr = ledger_keys(db)
    if lerr then db:release(); return lerr end

    local pending = {}
    for _, it in ipairs(items) do
        if not applied[it.key .. "\n" .. version] then pending[#pending + 1] = it end
    end
    if #pending == 0 then db:release(); return nil end

    local tx, tx_err = db:begin()
    if tx_err then db:release(); return "begin failed: " .. tostring(tx_err) end
    local ts = now()
    for _, it in ipairs(pending) do
        local aerr = it.apply(tx, ts)
        if not aerr then aerr = ledger_insert(tx, it.key, version, ts) end
        if aerr then tx:rollback(); db:release(); return aerr end
    end
    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return "commit failed: " .. tostring(commit_err) end
    return nil
end

-- ─── schema reads ────────────────────────────────────────────────────────

local function read_attributes(db)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_attributes ORDER BY sort_order ASC, code ASC", {})
    if qerr then return nil, qerr end
    local out = {}
    for _, r in ipairs(rows) do
        local attr = M.map_attribute(r)
        local orows = query(db, "SELECT * FROM spiralscout_pim_attribute_options WHERE attribute_code = $1 ORDER BY sort_order ASC, code ASC", { attr.code })
        local opts = {}
        for _, o in ipairs(orows or {}) do opts[#opts + 1] = map_option(o) end
        attr.options = opts
        out[#out + 1] = attr
    end
    return out, nil
end

local function read_families(db)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_families ORDER BY code ASC", {})
    if qerr then return nil, qerr end
    local out = {}
    for _, r in ipairs(rows) do
        local code = tostring(r.code or "")
        local arows = query(db, "SELECT attribute_code FROM spiralscout_pim_family_attributes WHERE family_code = $1 ORDER BY sort_order ASC", { code })
        local attrs = {}
        for _, a in ipairs(arows or {}) do attrs[#attrs + 1] = tostring(a.attribute_code or "") end
        local rreq = query(db, "SELECT channel_code, locale_code, required_attributes FROM spiralscout_pim_family_requirements WHERE family_code = $1", { code })
        local reqs = {}
        for _, rq in ipairs(rreq or {}) do
            reqs[#reqs + 1] = { channel_code = rq.channel_code, locale_code = rq.locale_code, required_attributes = decode(rq.required_attributes) or {} }
        end
        out[#out + 1] = {
            code = code,
            labels = decode(r.labels) or {},
            attribute_as_label = r.attribute_as_label,
            attribute_as_image = r.attribute_as_image,
            attributes = attrs,
            requirements = reqs,
            system = as_bool(r.system),
            created_by = r.created_by,
            updated_by = r.updated_by,
        }
    end
    return out, nil
end

local function read_channels(db)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_channels ORDER BY code ASC", {})
    if qerr then return nil, qerr end
    local out = {}
    for _, r in ipairs(rows) do out[#out + 1] = map_channel(r) end
    return out, nil
end

local function read_groups(db)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_attribute_groups ORDER BY sort_order ASC, code ASC", {})
    if qerr then return nil, qerr end
    local out = {}
    for _, r in ipairs(rows) do out[#out + 1] = map_group(r) end
    return out, nil
end

local function with_read(reader)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    local db, err = open()
    if not db then return nil, err end
    local out, qerr = reader(db)
    db:release()
    return out, qerr
end

function M.list_attributes() return with_read(read_attributes) end
function M.list_families() return with_read(read_families) end
function M.list_channels() return with_read(read_channels) end
function M.list_groups() return with_read(read_groups) end

function M.get_schema()
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    local db, err = open()
    if not db then return nil, err end
    local out = { version = config.version() }
    local readers = {
        { key = "channels", read = read_channels },
        { key = "attribute_groups", read = read_groups },
        { key = "attributes", read = read_attributes },
        { key = "families", read = read_families },
    }
    for _, r in ipairs(readers) do
        local rows, qerr = r.read(db)
        if not rows then db:release(); return nil, qerr end
        out[r.key] = rows
    end
    db:release()
    return out, nil
end

-- Attribute definitions keyed by code, shaped for value validation: the declared
-- attribute definition (config spread to top level, the shape attribute_config
-- expects) plus the enabled option codes and the enabled flag. The single source
-- the value write path validates against.
function M.value_validation_context()
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    local db, err = open()
    if not db then return nil, err end
    local attrs, aerr = read_attributes(db)
    db:release()
    if not attrs then return nil, aerr end
    local out = {}
    for _, attr in ipairs(attrs) do
        local option_codes = {}
        for _, o in ipairs(attr.options or {}) do
            if o.enabled ~= false then option_codes[#option_codes + 1] = o.code end
        end
        out[attr.code] = { def = M.to_definition(attr), options = option_codes, enabled = attr.enabled }
    end
    return out, nil
end

-- ─── attribute management ────────────────────────────────────────────────

local function fetch_attribute(db, code)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_attributes WHERE code = $1", { code })
    if qerr then return nil, qerr end
    if not rows[1] then return nil, "not found" end
    return M.map_attribute(rows[1]), nil
end

function M.get_attribute(code)
    local db, err = open()
    if not db then return nil, err end
    local attr, aerr = fetch_attribute(db, code)
    if not attr then db:release(); return nil, aerr end
    local orows = query(db, "SELECT * FROM spiralscout_pim_attribute_options WHERE attribute_code = $1 ORDER BY sort_order ASC, code ASC", { code })
    local opts = {}
    for _, o in ipairs(orows or {}) do opts[#opts + 1] = map_option(o) end
    attr.options = opts
    db:release()
    return attr, nil
end

function M.create_attribute(body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr, false end
    body = type(body) == "table" and body or {}
    local def = {}
    for k, v in pairs(body) do def[k] = v end
    def.system = nil
    def.options = nil
    local ok, verrors = attribute_config.validate_attribute(def)
    if not ok then return nil, errors_text(verrors), false end

    local db, err = open()
    if not db then return nil, err, false end
    local existing = query(db, "SELECT 1 FROM spiralscout_pim_attributes WHERE code = $1 LIMIT 1", { def.code })
    if existing[1] then db:release(); return nil, "an attribute with this code already exists", true end
    local ierr = insert_attribute(db, def, false, actor_id, now())
    db:release()
    if ierr then return nil, ierr, false end
    return M.get_attribute(def.code)
end

function M.update_attribute(code, body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    body = type(body) == "table" and body or {}

    local db, err = open()
    if not db then return nil, err end
    local attr, aerr = fetch_attribute(db, code)
    if not attr then db:release(); return nil, aerr end
    if body.type ~= nil and body.type ~= attr.type then
        db:release()
        return nil, "attribute type is immutable; create a new attribute instead"
    end

    local patch = {}
    for k, v in pairs(body) do patch[k] = v end
    patch.code = nil
    patch.type = nil
    patch.system = nil
    patch.options = nil
    local def = config.merge(M.to_definition(attr), patch)
    def.code = attr.code
    def.type = attr.type

    local ok, verrors = attribute_config.validate_attribute(def)
    if not ok then db:release(); return nil, errors_text(verrors) end

    local ts = now()
    local sets = {
        "labels = $1", "config = $2", "localizable = $3", "scopable = $4", "unique_value = $5",
        "searchable = $6", "filterable = $7", "enabled = $8", "sort_order = $9", "updated_at = $10",
    }
    local params = {
        encode(def.labels or {}), encode(split_config(def)), def.localizable == true, def.scopable == true,
        def.unique == true, def.searchable == true, def.filterable == true, def.enabled ~= false,
        tonumber(def.sort_order) or 0, ts,
    }
    if def.group == nil then
        sets[#sets + 1] = "attribute_group = NULL"
    else
        params[#params + 1] = def.group
        sets[#sets + 1] = "attribute_group = $" .. #params
    end
    if actor_id == nil then
        sets[#sets + 1] = "updated_by = NULL"
    else
        params[#params + 1] = actor_id
        sets[#sets + 1] = "updated_by = $" .. #params
    end
    params[#params + 1] = code
    local uerr = exec(db, "UPDATE spiralscout_pim_attributes SET " .. table.concat(sets, ", ") .. " WHERE code = $" .. #params, params)
    db:release()
    if uerr then return nil, uerr end
    return M.get_attribute(code)
end

local function disable_row(table_name, key_sql, key_params, actor_id)
    local db, err = open()
    if not db then return nil, err end
    local rows, qerr = query(db, "SELECT 1 FROM " .. table_name .. " WHERE " .. key_sql .. " LIMIT 1", key_params)
    if qerr then db:release(); return nil, qerr end
    if not rows[1] then db:release(); return nil, "not found" end

    local sets = { "enabled = $1", "updated_at = $2" }
    local params = { false, now() }
    if actor_id == nil then
        sets[#sets + 1] = "updated_by = NULL"
    else
        params[#params + 1] = actor_id
        sets[#sets + 1] = "updated_by = $" .. #params
    end
    local offset = #params
    local key_rebased = key_sql
    for i = #key_params, 1, -1 do
        key_rebased = key_rebased:gsub("%$" .. i, "$" .. (i + offset))
        params[i + offset] = key_params[i]
    end
    local uerr = exec(db, "UPDATE " .. table_name .. " SET " .. table.concat(sets, ", ") .. " WHERE " .. key_rebased, params)
    db:release()
    if uerr then return nil, uerr end
    return true, nil
end

function M.disable_attribute(code, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    local ok, err = disable_row("spiralscout_pim_attributes", "code = $1", { code }, actor_id)
    if not ok then return nil, err end
    return M.get_attribute(code)
end

-- ─── option management ───────────────────────────────────────────────────

local function fetch_option(db, attr_code, option_code)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_attribute_options WHERE attribute_code = $1 AND code = $2",
        { attr_code, option_code })
    if qerr then return nil, qerr end
    if not rows[1] then return nil, "not found" end
    return map_option(rows[1]), nil
end

function M.get_option(attr_code, option_code)
    local db, err = open()
    if not db then return nil, err end
    local opt, oerr = fetch_option(db, attr_code, option_code)
    db:release()
    return opt, oerr
end

function M.create_option(attr_code, body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr, false end
    body = type(body) == "table" and body or {}

    local db, err = open()
    if not db then return nil, err, false end
    local attr, aerr = fetch_attribute(db, attr_code)
    if not attr then db:release(); return nil, aerr, false end

    local option = {
        code = body.code,
        attribute_code = attr_code,
        labels = body.labels,
        aliases = body.aliases,
    }
    local ok, verrors = attribute_config.validate_option(option, { [attr_code] = attr })
    if not ok then db:release(); return nil, errors_text(verrors), false end

    local existing = query(db, "SELECT 1 FROM spiralscout_pim_attribute_options WHERE attribute_code = $1 AND code = $2 LIMIT 1",
        { attr_code, option.code })
    if existing[1] then db:release(); return nil, "an option with this code already exists", true end

    local ts = now()
    local ierr = exec(db, [[
        INSERT INTO spiralscout_pim_attribute_options
            (option_id, attribute_code, code, labels, aliases, enabled, system, sort_order, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
    ]], { new_id(), attr_code, option.code, encode(option.labels or {}), encode(option.aliases or {}),
        true, false, tonumber(body.sort_order) or 0, actor_id, actor_id, ts, ts })
    db:release()
    if ierr then return nil, ierr, false end
    return M.get_option(attr_code, option.code)
end

function M.update_option(attr_code, option_code, body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    body = type(body) == "table" and body or {}

    local db, err = open()
    if not db then return nil, err end
    local attr, aerr = fetch_attribute(db, attr_code)
    if not attr then db:release(); return nil, aerr end
    local existing, oerr = fetch_option(db, attr_code, option_code)
    if not existing then db:release(); return nil, oerr end

    local patch = {}
    for k, v in pairs(body) do patch[k] = v end
    patch.code = nil
    patch.system = nil
    local merged = config.merge({
        code = existing.code,
        attribute_code = attr_code,
        labels = existing.labels,
        aliases = existing.aliases,
        sort_order = existing.sort_order,
        enabled = existing.enabled,
    }, patch)
    merged.code = existing.code
    merged.attribute_code = attr_code

    local ok, verrors = attribute_config.validate_option(merged, { [attr_code] = attr })
    if not ok then db:release(); return nil, errors_text(verrors) end

    local ts = now()
    local sets = { "labels = $1", "aliases = $2", "sort_order = $3", "enabled = $4", "updated_at = $5" }
    local params = { encode(merged.labels or {}), encode(merged.aliases or {}), tonumber(merged.sort_order) or 0, merged.enabled ~= false, ts }
    if actor_id == nil then
        sets[#sets + 1] = "updated_by = NULL"
    else
        params[#params + 1] = actor_id
        sets[#sets + 1] = "updated_by = $" .. #params
    end
    params[#params + 1] = attr_code
    params[#params + 1] = option_code
    local uerr = exec(db, "UPDATE spiralscout_pim_attribute_options SET " .. table.concat(sets, ", ") ..
        " WHERE attribute_code = $" .. (#params - 1) .. " AND code = $" .. #params, params)
    db:release()
    if uerr then return nil, uerr end
    return M.get_option(attr_code, option_code)
end

function M.disable_option(attr_code, option_code, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    local ok, err = disable_row("spiralscout_pim_attribute_options", "attribute_code = $1 AND code = $2",
        { attr_code, option_code }, actor_id)
    if not ok then return nil, err end
    return M.get_option(attr_code, option_code)
end

-- ─── family management ───────────────────────────────────────────────────

local function fetch_family(db, code)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_families WHERE code = $1", { code })
    if qerr then return nil, qerr end
    if not rows[1] then return nil, "not found" end
    local fams, ferr = read_families(db)
    if not fams then return nil, ferr end
    for _, f in ipairs(fams) do
        if f.code == code then return f, nil end
    end
    return nil, "not found"
end

function M.get_family(code)
    local db, err = open()
    if not db then return nil, err end
    local fam, ferr = fetch_family(db, code)
    db:release()
    return fam, ferr
end

-- Requirements accepted with either channel/locale (declared shape) or
-- channel_code/locale_code (read shape) keys.
local function normalize_requirements(reqs)
    if type(reqs) ~= "table" then return nil end
    local out = {}
    for _, r in ipairs(reqs) do
        if type(r) == "table" then
            out[#out + 1] = {
                channel = r.channel or r.channel_code,
                locale = r.locale ~= nil and r.locale or r.locale_code,
                required_attributes = r.required_attributes,
            }
        end
    end
    return out
end

local function family_definition(fam)
    local reqs = {}
    for _, r in ipairs(fam.requirements or {}) do
        reqs[#reqs + 1] = { channel = r.channel_code, locale = r.locale_code, required_attributes = r.required_attributes }
    end
    return {
        code = fam.code,
        labels = fam.labels,
        attributes = fam.attributes,
        attribute_as_label = fam.attribute_as_label,
        attribute_as_image = fam.attribute_as_image,
        requirements = reqs,
    }
end

local function attribute_index(db)
    local attrs, aerr = read_attributes(db)
    if not attrs then return nil, aerr end
    return attribute_config.index_attributes(attrs), nil
end

function M.create_family(body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr, false end
    body = type(body) == "table" and body or {}
    local def = {
        code = body.code,
        labels = body.labels,
        attributes = body.attributes,
        attribute_as_label = body.attribute_as_label,
        attribute_as_image = body.attribute_as_image,
        requirements = normalize_requirements(body.requirements) or {},
    }

    local db, err = open()
    if not db then return nil, err, false end
    local index, ierr = attribute_index(db)
    if not index then db:release(); return nil, ierr, false end
    local ok, verrors = attribute_config.validate_family(def, index)
    if not ok then db:release(); return nil, errors_text(verrors), false end

    local existing = query(db, "SELECT 1 FROM spiralscout_pim_families WHERE code = $1 LIMIT 1", { def.code })
    if existing[1] then db:release(); return nil, "a family with this code already exists", true end

    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, "begin failed: " .. tostring(tx_err), false end
    local ts = now()
    local ferr = exec(tx, [[
        INSERT INTO spiralscout_pim_families (code, labels, attribute_as_label, attribute_as_image, config, system, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    ]], { def.code, encode(def.labels or {}), def.attribute_as_label, def.attribute_as_image, "{}", false, actor_id, actor_id, ts, ts })
    if not ferr then ferr = insert_family_rows(tx, def, ts) end
    if ferr then tx:rollback(); db:release(); return nil, ferr, false end
    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, "commit failed: " .. tostring(commit_err), false end
    return M.get_family(def.code)
end

-- Rewrites membership and requirement rows transactionally. Removing an
-- attribute from a family leaves its product values in place.
function M.update_family(code, body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    body = type(body) == "table" and body or {}

    local db, err = open()
    if not db then return nil, err end
    local fam, ferr = fetch_family(db, code)
    if not fam then db:release(); return nil, ferr end

    local patch = {}
    for k, v in pairs(body) do patch[k] = v end
    patch.code = nil
    patch.system = nil
    if patch.requirements ~= nil then patch.requirements = normalize_requirements(patch.requirements) end
    local def = config.merge(family_definition(fam), patch)
    def.code = code

    local index, ierr = attribute_index(db)
    if not index then db:release(); return nil, ierr end
    local ok, verrors = attribute_config.validate_family(def, index)
    if not ok then db:release(); return nil, errors_text(verrors) end

    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, "begin failed: " .. tostring(tx_err) end
    local ts = now()

    local sets = { "labels = $1", "updated_at = $2" }
    local params = { encode(def.labels or {}), ts }
    if def.attribute_as_label == nil then
        sets[#sets + 1] = "attribute_as_label = NULL"
    else
        params[#params + 1] = def.attribute_as_label
        sets[#sets + 1] = "attribute_as_label = $" .. #params
    end
    if def.attribute_as_image == nil then
        sets[#sets + 1] = "attribute_as_image = NULL"
    else
        params[#params + 1] = def.attribute_as_image
        sets[#sets + 1] = "attribute_as_image = $" .. #params
    end
    if actor_id == nil then
        sets[#sets + 1] = "updated_by = NULL"
    else
        params[#params + 1] = actor_id
        sets[#sets + 1] = "updated_by = $" .. #params
    end
    params[#params + 1] = code
    local uerr = exec(tx, "UPDATE spiralscout_pim_families SET " .. table.concat(sets, ", ") .. " WHERE code = $" .. #params, params)
    if not uerr then uerr = exec(tx, "DELETE FROM spiralscout_pim_family_attributes WHERE family_code = $1", { code }) end
    if not uerr then uerr = exec(tx, "DELETE FROM spiralscout_pim_family_requirements WHERE family_code = $1", { code }) end
    if not uerr then uerr = insert_family_rows(tx, def, ts) end
    if uerr then tx:rollback(); db:release(); return nil, uerr end
    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, "commit failed: " .. tostring(commit_err) end
    return M.get_family(code)
end

-- ─── channel management ──────────────────────────────────────────────────

local function fetch_channel(db, code)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_channels WHERE code = $1", { code })
    if qerr then return nil, qerr end
    if not rows[1] then return nil, "not found" end
    return map_channel(rows[1]), nil
end

function M.get_channel(code)
    local db, err = open()
    if not db then return nil, err end
    local ch, cerr = fetch_channel(db, code)
    db:release()
    return ch, cerr
end

function M.create_channel(body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr, false end
    body = type(body) == "table" and body or {}
    local def = { code = body.code, labels = body.labels, locales = body.locales, currencies = body.currencies }
    local ok, verrors = attribute_config.validate_channel(def)
    if not ok then return nil, errors_text(verrors), false end

    local db, err = open()
    if not db then return nil, err, false end
    local existing = query(db, "SELECT 1 FROM spiralscout_pim_channels WHERE code = $1 LIMIT 1", { def.code })
    if existing[1] then db:release(); return nil, "a channel with this code already exists", true end
    local ts = now()
    local ierr = exec(db, [[
        INSERT INTO spiralscout_pim_channels (code, labels, locales, currencies, enabled, system, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    ]], { def.code, encode(def.labels or {}), encode(def.locales or {}), encode(def.currencies or {}), true, false, actor_id, actor_id, ts, ts })
    db:release()
    if ierr then return nil, ierr, false end
    return M.get_channel(def.code)
end

function M.update_channel(code, body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    body = type(body) == "table" and body or {}

    local db, err = open()
    if not db then return nil, err end
    local existing, cerr = fetch_channel(db, code)
    if not existing then db:release(); return nil, cerr end

    local patch = {}
    for k, v in pairs(body) do patch[k] = v end
    patch.code = nil
    patch.system = nil
    local merged = config.merge({
        code = existing.code,
        labels = existing.labels,
        locales = existing.locales,
        currencies = existing.currencies,
        enabled = existing.enabled,
    }, patch)
    merged.code = existing.code

    local ok, verrors = attribute_config.validate_channel(merged)
    if not ok then db:release(); return nil, errors_text(verrors) end

    local ts = now()
    local sets = { "labels = $1", "locales = $2", "currencies = $3", "enabled = $4", "updated_at = $5" }
    local params = { encode(merged.labels or {}), encode(merged.locales or {}), encode(merged.currencies or {}), merged.enabled ~= false, ts }
    if actor_id == nil then
        sets[#sets + 1] = "updated_by = NULL"
    else
        params[#params + 1] = actor_id
        sets[#sets + 1] = "updated_by = $" .. #params
    end
    params[#params + 1] = code
    local uerr = exec(db, "UPDATE spiralscout_pim_channels SET " .. table.concat(sets, ", ") .. " WHERE code = $" .. #params, params)
    db:release()
    if uerr then return nil, uerr end
    return M.get_channel(code)
end

function M.disable_channel(code, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    local ok, err = disable_row("spiralscout_pim_channels", "code = $1", { code }, actor_id)
    if not ok then return nil, err end
    return M.get_channel(code)
end

-- ─── attribute group management ──────────────────────────────────────────

local function fetch_group(db, code)
    local rows, qerr = query(db, "SELECT * FROM spiralscout_pim_attribute_groups WHERE code = $1", { code })
    if qerr then return nil, qerr end
    if not rows[1] then return nil, "not found" end
    return map_group(rows[1]), nil
end

function M.get_group(code)
    local db, err = open()
    if not db then return nil, err end
    local g, gerr = fetch_group(db, code)
    db:release()
    return g, gerr
end

function M.create_group(body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr, false end
    body = type(body) == "table" and body or {}
    local def = { code = body.code, labels = body.labels, sort_order = body.sort_order }
    local ok, verrors = attribute_config.validate_group(def)
    if not ok then return nil, errors_text(verrors), false end

    local db, err = open()
    if not db then return nil, err, false end
    local existing = query(db, "SELECT 1 FROM spiralscout_pim_attribute_groups WHERE code = $1 LIMIT 1", { def.code })
    if existing[1] then db:release(); return nil, "an attribute group with this code already exists", true end
    local ts = now()
    local ierr = exec(db, [[
        INSERT INTO spiralscout_pim_attribute_groups (code, labels, sort_order, system, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    ]], { def.code, encode(def.labels or {}), tonumber(def.sort_order) or 0, false, actor_id, actor_id, ts, ts })
    db:release()
    if ierr then return nil, ierr, false end
    return M.get_group(def.code)
end

function M.update_group(code, body, actor_id)
    local rerr = M.reconcile()
    if rerr then return nil, rerr end
    body = type(body) == "table" and body or {}

    local db, err = open()
    if not db then return nil, err end
    local existing, gerr = fetch_group(db, code)
    if not existing then db:release(); return nil, gerr end

    local patch = {}
    for k, v in pairs(body) do patch[k] = v end
    patch.code = nil
    patch.system = nil
    local merged = config.merge({ code = existing.code, labels = existing.labels, sort_order = existing.sort_order }, patch)
    merged.code = existing.code

    local ok, verrors = attribute_config.validate_group(merged)
    if not ok then db:release(); return nil, errors_text(verrors) end

    local ts = now()
    local sets = { "labels = $1", "sort_order = $2", "updated_at = $3" }
    local params = { encode(merged.labels or {}), tonumber(merged.sort_order) or 0, ts }
    if actor_id == nil then
        sets[#sets + 1] = "updated_by = NULL"
    else
        params[#params + 1] = actor_id
        sets[#sets + 1] = "updated_by = $" .. #params
    end
    params[#params + 1] = code
    local uerr = exec(db, "UPDATE spiralscout_pim_attribute_groups SET " .. table.concat(sets, ", ") .. " WHERE code = $" .. #params, params)
    db:release()
    if uerr then return nil, uerr end
    return M.get_group(code)
end

return M
