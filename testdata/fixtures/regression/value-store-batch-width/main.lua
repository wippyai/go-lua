local funcs = require("funcs")
local fs = require("fs")
local json = require("json")
local sql = require("sql")
local test = require("test")
local time = require("time")

local M = {}

local APP_DB = "app:db"
local LOAD_HELPER = "app:value_store_bench_load"
local RESULTS_PATH = "/bench/RESULTS.md"
local CRM_ID = "s0-bench"
local COLLECTION_ID = "sales_pipeline"
local UPDATED_AT = "2026-07-08T00:00:00Z"
local Q1_CUTOFF = "2026-06-01T00:00:00Z"
local PERSON_COUNT = 100000
local OPPORTUNITY_COUNT = 20000
local SAMPLE_COUNT = 7
local WARMUP_COUNT = 2
local SQLITE_BAR_MS = 500
local POSTGRES_BAR_MS = 300

local STAGES = { "Lead", "Qualified", "Demo", "Won", "Lost" }
local FAST_TEST_FILTERS = {
    "wait_for_boot",
    "projection_test",
    "constraints_test",
    "validation_test",
    "reader_test",
    "packaging_test",
    "access_test",
    "checkpoint_test",
    "sink_test",
}

local TABLES = {
    "spiralscout_crm_collection_membership",
    "spiralscout_crm_reconciliation",
    "spiralscout_crm_record_value_index",
    "spiralscout_crm_attribute",
    "spiralscout_crm_view",
    "spiralscout_crm_pipeline",
    "spiralscout_crm_object",
}

local PERSON_ATTRS = {
    { "name", "text" },
    { "email", "email" },
    { "phone", "phone" },
    { "status", "select" },
    { "owner", "text" },
    { "score", "number" },
    { "last_touch_at", "date" },
    { "company_name", "text" },
    { "tags", "multi_select" },
    { "vertical", "text" },
    { "created_at", "date" },
    { "opted_in", "boolean" },
}

local OPPORTUNITY_ATTRS = {
    { "name", "text" },
    { "company", "text" },
    { "amount", "currency" },
    { "close_date", "date" },
    { "owner", "text" },
    { "vertical", "text" },
    { "score", "number" },
    { "stage", "select" },
}

local function fail(message)
    error(tostring(message), 2)
end

local function check_err(err, context)
    if err then
        fail((context or "operation failed") .. ": " .. tostring(err))
    end
end

local function execute(db, statement, params, context)
    local _, err = db:execute(statement, params or {})
    check_err(err, context or statement)
end

local function query(db, statement, params, context)
    local rows, err = db:query(statement, params or {})
    check_err(err, context or statement)
    return rows or {}
end

local function elapsed_ms(started)
    return time.now():sub(started):milliseconds()
end

local function percentile(values, pct)
    if #values == 0 then
        return 0
    end
    local index = math.ceil((pct / 100) * #values)
    if index < 1 then
        index = 1
    elseif index > #values then
        index = #values
    end
    return values[index]
end

local function fmt_ms(value)
    if value == nil then
        return "-"
    end
    return string.format("%.1f", tonumber(value) or 0)
end

local function db_dialect(db)
    local ok, db_type = pcall(function()
        return db:type()
    end)
    if ok then
        if sql.type and db_type == sql.type.POSTGRES then
            return "postgres"
        end
        if sql.type and db_type == sql.type.SQLITE then
            return "sqlite"
        end
        local as_text = tostring(db_type or "")
        if as_text:find("postgres", 1, true) then
            return "postgres"
        end
        if as_text:find("sqlite", 1, true) then
            return "sqlite"
        end
    end

    local sqlite_probe, sqlite_err = db:query("SELECT sqlite_version() AS version", {})
    if not sqlite_err and sqlite_probe then
        return "sqlite"
    end

    local postgres_probe, postgres_err = db:query("SELECT current_database() AS name", {})
    if not postgres_err and postgres_probe then
        return "postgres"
    end
    return "unknown"
end

local function open_db()
    local db, err = sql.get(APP_DB)
    check_err(err, "open " .. APP_DB)
    if not db then
        fail(APP_DB .. " unavailable")
    end
    return db
end

local function wait_for_tables()
    for _ = 1, 300 do
        local db, err = sql.get(APP_DB)
        if not err and db then
            -- Probe the table itself: engine-agnostic; a query error means the
            -- migration has not created it yet.
            local _, qerr = db:query("SELECT 1 FROM spiralscout_crm_record_value_index LIMIT 1", {})
            db:release()
            if not qerr then
                return
            end
        elseif db then
            db:release()
        end
        time.sleep("100ms")
    end
    fail("bootloader did not create CRM tables before benchmark start")
end

local function placeholder_row(width, offset)
    local parts = {}
    for i = 1, width do
        parts[i] = "$" .. tostring(offset + i)
    end
    return "(" .. table.concat(parts, ", ") .. ")"
end

local function flush_batch(batch)
    if batch.count == 0 then
        return
    end
    local rows = {}
    for i = 1, batch.count do
        rows[i] = placeholder_row(batch.width, (i - 1) * batch.width)
    end
    execute(batch.db, batch.prefix .. table.concat(rows, ", "), batch.params, batch.context)
    batch.count = 0
    batch.param_count = 0
    batch.params = {}
end

local function new_batch(db, prefix, width, max_rows, context)
    return {
        db = db,
        prefix = prefix,
        width = width,
        max_rows = max_rows,
        count = 0,
        param_count = 0,
        params = {},
        context = context,
    }
end

local function batch_add(batch, row)
    for i = 1, batch.width do
        batch.param_count = batch.param_count + 1
        batch.params[batch.param_count] = row[i]
    end
    batch.count = batch.count + 1
    if batch.count >= batch.max_rows then
        flush_batch(batch)
    end
end

local function lanes(value)
    local kind = type(value)
    if kind == "number" then
        return tostring(value), value, nil, nil
    end
    if kind == "boolean" then
        return tostring(value), nil, nil, value
    end
    if kind == "string" then
        if value:match("^%d%d%d%d%-%d%d%-%d%d") then
            return value, nil, value, nil
        end
        return value, nil, nil, nil
    end
    if value == nil then
        return nil, nil, nil, nil
    end
    return json.encode(value), nil, nil, nil
end

local function add_value(batch, record_id, object_type, attr, value)
    local value_text, value_num, value_time, value_bool = lanes(value)
    batch_add(batch, {
        CRM_ID,
        record_id,
        object_type,
        attr,
        value_text,
        value_num,
        value_time,
        value_bool,
        UPDATED_AT,
    })
end

local function iso_date(seed, base_year)
    local month = (seed % 12) + 1
    local day = (seed % 28) + 1
    return string.format("%04d-%02d-%02dT00:00:00Z", base_year, month, day)
end

local function person_id(i)
    return string.format("person:%06d", i)
end

local function opportunity_id(i)
    return string.format("opportunity:%06d", i)
end

local function seed_schema(tx)
    execute(tx, [[
        INSERT INTO spiralscout_crm_object
            (crm_id, object_type, label, icon, plural, is_syncable, updated_at)
        VALUES
            ($1, 'person', 'Person', 'tabler:user', 'People', $2, $3),
            ($4, 'opportunity', 'Opportunity', 'tabler:currency-dollar', 'Opportunities', $5, $6)
    ]], { CRM_ID, true, UPDATED_AT, CRM_ID, true, UPDATED_AT }, "seed objects")

    local attr_batch = new_batch(tx, [[
        INSERT INTO spiralscout_crm_attribute
            (crm_id, object_type, attr, data_type, label, config, sort_order, system, updated_at)
        VALUES
    ]], 9, 80, "seed attributes")

    local order = 10
    for _, def in ipairs(PERSON_ATTRS) do
        batch_add(attr_batch, {
            CRM_ID, "person", def[1], def[2], def[1], "{}", order, true, UPDATED_AT,
        })
        order = order + 10
    end
    order = 10
    for _, def in ipairs(OPPORTUNITY_ATTRS) do
        batch_add(attr_batch, {
            CRM_ID, "opportunity", def[1], def[2], def[1], "{}", order, true, UPDATED_AT,
        })
        order = order + 10
    end
    flush_batch(attr_batch)

    execute(tx, [[
        INSERT INTO spiralscout_crm_pipeline
            (crm_id, collection_id, object_type, label, stages, updated_at)
        VALUES ($1, $2, 'opportunity', 'Sales Pipeline', $3, $4)
    ]], {
        CRM_ID,
        COLLECTION_ID,
        json.encode({
            { stage_id = "Lead", label = "Lead", order = 10 },
            { stage_id = "Qualified", label = "Qualified", order = 20 },
            { stage_id = "Demo", label = "Demo", order = 30 },
            { stage_id = "Won", label = "Won", order = 40 },
            { stage_id = "Lost", label = "Lost", order = 50 },
        }),
        UPDATED_AT,
    }, "seed pipeline")
end

local function seed_people(value_batch, recon_batch)
    local first_names = { "Ada", "Grace", "Linus", "Katherine", "Donald", "Barbara", "Margaret", "Ken" }
    local last_names = { "Stone", "Kim", "Singh", "Garcia", "Jones", "Brown", "Davis", "Miller" }
    local company_roots = { "Northwind", "Aperture", "Globex", "Initech", "Umbrella", "Wayne", "Stark", "Wonka" }

    for i = 1, PERSON_COUNT do
        local id = person_id(i)
        local love = i % 23 == 0
        local first = love and "Love" or first_names[(i % #first_names) + 1]
        local last = love and "Lovelace" or last_names[(math.floor(i / 7) % #last_names) + 1]
        local company = company_roots[(i % #company_roots) + 1] .. " " .. string.format("%04d", i % 2000)
        local status
        if i % 10 < 4 then
            status = "Lead"
        elseif i % 10 < 7 then
            status = "Qualified"
        elseif i % 10 < 9 then
            status = "Customer"
        else
            status = "Inactive"
        end
        local owner = (i % 5 == 0) and "maria" or "john"
        local tags = { "newsletter", (i % 3 == 0) and "enterprise" or "startup" }
        if i % 11 == 0 then
            tags[#tags + 1] = "vip"
        end

        add_value(value_batch, id, "person", "name", first .. " " .. last .. " " .. string.format("%06d", i))
        add_value(value_batch, id, "person", "email", "person" .. tostring(i) .. "@example.test")
        add_value(value_batch, id, "person", "phone", "+1555" .. string.format("%07d", i))
        add_value(value_batch, id, "person", "status", status)
        add_value(value_batch, id, "person", "owner", owner)
        add_value(value_batch, id, "person", "score", (i * 37) % 101)
        add_value(value_batch, id, "person", "last_touch_at", iso_date(i, 2026))
        add_value(value_batch, id, "person", "company_name", company)
        add_value(value_batch, id, "person", "tags", tags)
        add_value(value_batch, id, "person", "vertical", (i % 4 == 0) and "kickside" or "general")
        add_value(value_batch, id, "person", "created_at", iso_date(i, 2025))
        add_value(value_batch, id, "person", "opted_in", i % 2 == 0)

        batch_add(recon_batch, {
            CRM_ID,
            id,
            "person",
            "bench_email",
            "person" .. tostring(i) .. "@example.test",
            id,
            nil,
            UPDATED_AT,
        })
    end
end

local function seed_opportunities(value_batch, membership_batch)
    local companies = { "Northwind", "Aperture", "Globex", "Initech", "Umbrella", "Wayne", "Stark", "Wonka" }
    for i = 1, OPPORTUNITY_COUNT do
        local id = opportunity_id(i)
        local stage = STAGES[((i - 1) % #STAGES) + 1]
        local owner = (i % 11 == 0) and "maria" or "john"
        local vertical = (i % 3 == 0) and "general" or "kickside"
        local score = (i * 53) % 101
        local amount = 1000 + ((i * 7919) % 250000)
        local company = companies[(i % #companies) + 1] .. " " .. string.format("%04d", i % 2000)

        add_value(value_batch, id, "opportunity", "name", "Expansion " .. string.format("%06d", i))
        add_value(value_batch, id, "opportunity", "company", company)
        add_value(value_batch, id, "opportunity", "amount", amount)
        add_value(value_batch, id, "opportunity", "close_date", iso_date(i + 90, 2026))
        add_value(value_batch, id, "opportunity", "owner", owner)
        add_value(value_batch, id, "opportunity", "vertical", vertical)
        add_value(value_batch, id, "opportunity", "score", score)
        add_value(value_batch, id, "opportunity", "stage", stage)

        batch_add(membership_batch, {
            CRM_ID,
            COLLECTION_ID,
            id,
            stage,
            i,
            UPDATED_AT,
        })
    end
end

local function cleanup(db)
    for _, table_name in ipairs(TABLES) do
        execute(db, "DELETE FROM " .. table_name .. " WHERE crm_id = $1", { CRM_ID }, "cleanup " .. table_name)
    end
end

local function seed_fixture(db)
    cleanup(db)

    local started = time.now()
    local tx, tx_err = db:begin()
    check_err(tx_err, "begin seed transaction")

    local ok, err = pcall(function()
        seed_schema(tx)

        local value_batch = new_batch(tx, [[
            INSERT INTO spiralscout_crm_record_value_index
                (crm_id, record_id, object_type, attr, value_text, value_num, value_time, value_bool, updated_at)
            VALUES
        ]], 9, 80, "seed value index")
        local recon_batch = new_batch(tx, [[
            INSERT INTO spiralscout_crm_reconciliation
                (crm_id, record_id, object_type, external_source, external_id, canonical_id, superseded_at, updated_at)
            VALUES
        ]], 8, 100, "seed reconciliation")
        local membership_batch = new_batch(tx, [[
            INSERT INTO spiralscout_crm_collection_membership
                (crm_id, collection_id, record_id, stage_id, position, updated_at)
            VALUES
        ]], 6, 120, "seed memberships")

        seed_people(value_batch, recon_batch)
        seed_opportunities(value_batch, membership_batch)

        flush_batch(value_batch)
        flush_batch(recon_batch)
        flush_batch(membership_batch)
    end)

    if not ok then
        pcall(function()
            tx:rollback()
        end)
        fail(err)
    end

    local _, commit_err = tx:commit()
    check_err(commit_err, "commit seed transaction")
    pcall(function()
        db:execute("ANALYZE", {})
    end)

    local people = query(db, [[
        SELECT COUNT(*) AS n
        FROM spiralscout_crm_record_value_index
        WHERE crm_id = $1 AND object_type = 'person' AND attr = 'name'
    ]], { CRM_ID }, "count people")
    local opportunities = query(db, [[
        SELECT COUNT(*) AS n
        FROM spiralscout_crm_record_value_index
        WHERE crm_id = $1 AND object_type = 'opportunity' AND attr = 'name'
    ]], { CRM_ID }, "count opportunities")

    return {
        seed_ms = elapsed_ms(started),
        people = tonumber(people[1] and people[1].n) or 0,
        opportunities = tonumber(opportunities[1] and opportunities[1].n) or 0,
    }
end

local Q1_SQL = [[
    SELECT status.record_id, company_name.value_text AS company_name, score.value_num AS score
    FROM spiralscout_crm_record_value_index status
    JOIN spiralscout_crm_record_value_index owner
      ON owner.crm_id = status.crm_id AND owner.record_id = status.record_id
     AND owner.attr = 'owner' AND owner.value_text = 'john'
    JOIN spiralscout_crm_record_value_index score
      ON score.crm_id = status.crm_id AND score.record_id = status.record_id
     AND score.attr = 'score' AND score.value_num >= 70
    JOIN spiralscout_crm_record_value_index last_touch
      ON last_touch.crm_id = status.crm_id AND last_touch.record_id = status.record_id
     AND last_touch.attr = 'last_touch_at' AND last_touch.value_time < $1
    JOIN spiralscout_crm_record_value_index company_name
      ON company_name.crm_id = status.crm_id AND company_name.record_id = status.record_id
     AND company_name.attr = 'company_name'
    WHERE status.crm_id = $2 AND status.object_type = 'person'
      AND status.attr = 'status' AND status.value_text IN ('Lead', 'Qualified')
    ORDER BY company_name.value_text ASC, score.value_num DESC
    LIMIT 50 OFFSET 5000
]]

local Q2_SQL = [[
    SELECT name.record_id, name.value_text AS name
    FROM spiralscout_crm_record_value_index name
    WHERE name.crm_id = $1 AND name.object_type = 'person'
      AND name.attr = 'name' AND LOWER(name.value_text) LIKE $2
    ORDER BY name.value_text ASC
    LIMIT 50 OFFSET 100
]]

local Q3_SQL = [[
    SELECT tags.record_id, name.value_text AS name
    FROM spiralscout_crm_record_value_index tags
    JOIN spiralscout_crm_record_value_index name
      ON name.crm_id = tags.crm_id AND name.record_id = tags.record_id
     AND name.attr = 'name'
    WHERE tags.crm_id = $1 AND tags.object_type = 'person'
      AND tags.attr = 'tags' AND tags.value_text LIKE $2
    ORDER BY name.value_text ASC
    LIMIT 50 OFFSET 100
]]

local Q4_STAGE_SQL = [[
    SELECT m.record_id, m.stage_id, name.value_text AS name, company.value_text AS company,
           amount.value_num AS amount, close_date.value_time AS close_date, score.value_num AS score
    FROM spiralscout_crm_collection_membership m
    JOIN spiralscout_crm_record_value_index owner
      ON owner.crm_id = m.crm_id AND owner.record_id = m.record_id
     AND owner.attr = 'owner' AND owner.value_text = 'john'
    JOIN spiralscout_crm_record_value_index vertical
      ON vertical.crm_id = m.crm_id AND vertical.record_id = m.record_id
     AND vertical.attr = 'vertical' AND vertical.value_text = 'kickside'
    JOIN spiralscout_crm_record_value_index score
      ON score.crm_id = m.crm_id AND score.record_id = m.record_id
     AND score.attr = 'score'
    JOIN spiralscout_crm_record_value_index name
      ON name.crm_id = m.crm_id AND name.record_id = m.record_id
     AND name.attr = 'name'
    JOIN spiralscout_crm_record_value_index company
      ON company.crm_id = m.crm_id AND company.record_id = m.record_id
     AND company.attr = 'company'
    JOIN spiralscout_crm_record_value_index amount
      ON amount.crm_id = m.crm_id AND amount.record_id = m.record_id
     AND amount.attr = 'amount'
    JOIN spiralscout_crm_record_value_index close_date
      ON close_date.crm_id = m.crm_id AND close_date.record_id = m.record_id
     AND close_date.attr = 'close_date'
    WHERE m.crm_id = $1 AND m.collection_id = $2 AND m.stage_id = $3
    ORDER BY score.value_num DESC
    LIMIT 25
]]

local Q5_SQL = [[
    SELECT m.stage_id, COUNT(*) AS record_count, SUM(amount.value_num) AS amount_sum
    FROM spiralscout_crm_collection_membership m
    JOIN spiralscout_crm_record_value_index owner
      ON owner.crm_id = m.crm_id AND owner.record_id = m.record_id
     AND owner.attr = 'owner' AND owner.value_text = 'john'
    JOIN spiralscout_crm_record_value_index vertical
      ON vertical.crm_id = m.crm_id AND vertical.record_id = m.record_id
     AND vertical.attr = 'vertical' AND vertical.value_text = 'kickside'
    JOIN spiralscout_crm_record_value_index amount
      ON amount.crm_id = m.crm_id AND amount.record_id = m.record_id
     AND amount.attr = 'amount'
    WHERE m.crm_id = $1 AND m.collection_id = $2
    GROUP BY m.stage_id
    ORDER BY m.stage_id
]]

local Q6_GET_SQL = [[
    SELECT record_id, object_type, attr, value_text, value_num, value_time, value_bool
    FROM spiralscout_crm_record_value_index
    WHERE crm_id = $1 AND record_id = $2
]]

local Q6_ALIAS_SQL = [[
    SELECT record_id
    FROM spiralscout_crm_reconciliation
    WHERE crm_id = $1 AND external_source = $2 AND external_id = $3
]]

local function run_q1(db)
    return #query(db, Q1_SQL, { Q1_CUTOFF, CRM_ID }, "Q1")
end

local function run_q2(db)
    return #query(db, Q2_SQL, { CRM_ID, "%love%" }, "Q2")
end

local function run_q3(db)
    return #query(db, Q3_SQL, { CRM_ID, "%\"enterprise\"%" }, "Q3")
end

local function run_q4(db)
    local count = 0
    for _, stage in ipairs(STAGES) do
        local rows = query(db, Q4_STAGE_SQL, { CRM_ID, COLLECTION_ID, stage }, "Q4 " .. stage)
        count = count + #rows
    end
    return count
end

local function run_q5(db)
    return #query(db, Q5_SQL, { CRM_ID, COLLECTION_ID }, "Q5")
end

local function run_q6_once(db, i)
    local id = person_id(((i * 7919) % PERSON_COUNT) + 1)
    local email = "person" .. tostring(((i * 7919) % PERSON_COUNT) + 1) .. "@example.test"
    query(db, Q6_GET_SQL, { CRM_ID, id }, "Q6 get")
    query(db, Q6_ALIAS_SQL, { CRM_ID, "bench_email", email }, "Q6 alias")
end

local function measure_query(fn)
    for _ = 1, WARMUP_COUNT do
        fn()
    end
    local samples = {}
    local rows = 0
    for i = 1, SAMPLE_COUNT do
        local started = time.now()
        rows = fn()
        samples[i] = elapsed_ms(started)
    end
    table.sort(samples)
    return {
        samples = SAMPLE_COUNT,
        rows = rows,
        min_ms = samples[1] or 0,
        p50_ms = percentile(samples, 50),
        p95_ms = percentile(samples, 95),
        max_ms = samples[#samples] or 0,
    }
end

local function measure_q6(db)
    local future, ferr = funcs.async(LOAD_HELPER, { loops = 4 })
    check_err(ferr, "start Q6 load helper")
    time.sleep("25ms")

    local samples = {}
    for i = 1, 120 do
        local started = time.now()
        run_q6_once(db, i)
        samples[i] = elapsed_ms(started)
    end

    local _, ok = future:response():receive()
    if not ok then
        fail("Q6 load helper response channel closed")
    end
    local payload, result_err = future:result()
    if result_err then
        fail("Q6 load helper failed: " .. tostring(result_err))
    end
    local data = payload and payload:data() or nil
    if type(data) == "table" and data.ok == false then
        fail("Q6 load helper failed: " .. tostring(data.error))
    end
    table.sort(samples)

    return {
        samples = #samples,
        rows = #samples,
        min_ms = samples[1] or 0,
        p50_ms = percentile(samples, 50),
        p95_ms = percentile(samples, 95),
        max_ms = samples[#samples] or 0,
    }
end

local function explain_sqlite(db, statement, params)
    local rows, err = db:query("EXPLAIN QUERY PLAN " .. statement, params or {})
    if err then
        return "EXPLAIN failed: " .. tostring(err), true
    end
    local details = {}
    local spill = false
    for _, row in ipairs(rows or {}) do
        local detail = tostring(row.detail or row[4] or "")
        if detail ~= "" then
            details[#details + 1] = detail
            if detail:find("USE TEMP B-TREE", 1, true) then
                spill = true
            end
        end
    end
    if #details == 0 then
        details[1] = "no detail"
    end
    return table.concat(details, " | "), spill
end

local function sqlite_plan_for(db, id)
    if id == "Q1" then
        return explain_sqlite(db, Q1_SQL, { Q1_CUTOFF, CRM_ID })
    elseif id == "Q2" then
        return explain_sqlite(db, Q2_SQL, { CRM_ID, "%love%" })
    elseif id == "Q3" then
        return explain_sqlite(db, Q3_SQL, { CRM_ID, "%\"enterprise\"%" })
    elseif id == "Q4" then
        return explain_sqlite(db, Q4_STAGE_SQL, { CRM_ID, COLLECTION_ID, STAGES[1] })
    elseif id == "Q5" then
        return explain_sqlite(db, Q5_SQL, { CRM_ID, COLLECTION_ID })
    elseif id == "Q6" then
        local get_detail, get_spill = explain_sqlite(db, Q6_GET_SQL, { CRM_ID, person_id(1) })
        local alias_detail, alias_spill = explain_sqlite(db, Q6_ALIAS_SQL, {
            CRM_ID,
            "bench_email",
            "person1@example.test",
        })
        return "get: " .. get_detail .. " / alias: " .. alias_detail, get_spill or alias_spill
    end
    return "not checked", false
end

local QUERIES = {
    {
        id = "Q1",
        shape = "table multi-attr filter + cross-attr sort + deep page",
        concrete = "status IN ('Lead','Qualified') AND owner='john' AND score >= 70 AND last_touch_at < ? ORDER BY company_name ASC, score DESC LIMIT 50 OFFSET 5000",
        run = run_q1,
    },
    {
        id = "Q2",
        shape = "table contains",
        concrete = "name contains 'love' + sort + page",
        run = run_q2,
    },
    {
        id = "Q3",
        shape = "table multi_select membership",
        concrete = "tags has 'enterprise'",
        run = run_q3,
    },
    {
        id = "Q4",
        shape = "board cards with per-stage pagination",
        concrete = "sales_pipeline board, filter owner='john' AND vertical='kickside', card fields name,company,amount,close_date, sort score DESC, per-stage LIMIT 25",
        run = run_q4,
    },
    {
        id = "Q5",
        shape = "board aggregates",
        concrete = "per-stage COUNT(*) + SUM(amount) under the Q4 filter",
        run = run_q5,
    },
    {
        id = "Q6",
        shape = "exists/get under load",
        concrete = "get_record + alias lookup p95 while Q1 runs",
        run = measure_q6,
        measured = true,
    },
}

local function verdict_for(dialect, measurement, spill)
    local bar = dialect == "postgres" and POSTGRES_BAR_MS or SQLITE_BAR_MS
    local pass_time = (measurement.p95_ms or 0) < bar
    if dialect == "sqlite" and spill then
        return "FAIL", "sqlite temp B-tree spill"
    end
    if not pass_time then
        return "FAIL", "p95 >= " .. tostring(bar) .. "ms"
    end
    return "PASS", ""
end

local function decision_for(row)
    if row.dialect == "postgres" and row.verdict == "PENDING" then
        return "Postgres pending; no reachable configured database."
    end
    if row.verdict == "PASS" then
        return "EAV sufficient for this fixture and dialect."
    end
    if row.query == "Q2" then
        return "EAV contains path not accepted; record FTS/trigram search-index decision before S9."
    end
    if row.query == "Q3" then
        return "Hybrid/normalized membership lane needed for multi_select filtering."
    end
    return "Hybrid hot-columns/read-cache needed for this shape."
end

local function benchmark_current_db(db, dialect)
    local rows = {}
    for _, q in ipairs(QUERIES) do
        local plan, spill = "not checked", false
        if dialect == "sqlite" then
            plan, spill = sqlite_plan_for(db, q.id)
        end
        local measurement
        if q.measured then
            measurement = q.run(db)
        else
            measurement = measure_query(function()
                return q.run(db)
            end)
        end
        local verdict, reason = verdict_for(dialect, measurement, spill)
        local row = {
            query = q.id,
            shape = q.shape,
            concrete = q.concrete,
            dialect = dialect,
            samples = measurement.samples,
            rows = measurement.rows,
            p50_ms = measurement.p50_ms,
            p95_ms = measurement.p95_ms,
            max_ms = measurement.max_ms,
            sqlite_spill = dialect == "sqlite" and spill or false,
            sqlite_plan = plan,
            verdict = verdict,
            reason = reason,
        }
        row.decision = decision_for(row)
        rows[#rows + 1] = row
    end
    return rows
end

local function pending_rows(dialect)
    local rows = {}
    for _, q in ipairs(QUERIES) do
        local row = {
            query = q.id,
            shape = q.shape,
            concrete = q.concrete,
            dialect = dialect,
            samples = "-",
            rows = "-",
            p50_ms = nil,
            p95_ms = nil,
            max_ms = nil,
            sqlite_spill = false,
            sqlite_plan = dialect == "sqlite" and "PENDING" or "not applicable",
            verdict = "PENDING",
            reason = "no reachable configured " .. dialect .. " database",
        }
        row.decision = decision_for(row)
        rows[#rows + 1] = row
    end
    return rows
end

local function markdown_escape(value)
    return tostring(value or ""):gsub("|", "\\|"):gsub("\n", " ")
end

local function result_table(rows)
    local out = {
        "| Query | Shape | Dialect | Samples | Rows | p50 ms | p95 ms | Max ms | SQLite Temp B-tree | Verdict | Decision |",
        "|---|---|---:|---:|---:|---:|---:|---:|---|---|---|",
    }
    for _, row in ipairs(rows) do
        out[#out + 1] = table.concat({
            "| " .. row.query,
            markdown_escape(row.shape),
            row.dialect,
            tostring(row.samples),
            tostring(row.rows),
            fmt_ms(row.p50_ms),
            fmt_ms(row.p95_ms),
            fmt_ms(row.max_ms),
            row.dialect == "sqlite" and (row.sqlite_spill and "YES" or "NO") or "n/a",
            row.verdict .. (row.reason ~= "" and (" (" .. row.reason .. ")") or ""),
            markdown_escape(row.decision) .. " |",
        }, " | ")
    end
    return table.concat(out, "\n")
end

local function decision_block(rows)
    local out = {
        "## Recorded Decision",
        "",
    }
    for _, row in ipairs(rows) do
        out[#out + 1] = "- " .. row.query .. " / " .. row.dialect .. ": " .. row.decision
    end
    out[#out + 1] = ""
    local q145_failed = false
    for _, row in ipairs(rows) do
        if row.dialect ~= "postgres" and row.verdict == "FAIL"
            and (row.query == "Q1" or row.query == "Q4" or row.query == "Q5") then
            q145_failed = true
        end
    end
    if q145_failed then
        out[#out + 1] = "Overall: SQL pushdown over the current EAV lanes is not sufficient for the S0 acceptance bar. The S9 hybrid hot-column/read-cache path is authorized for saved-view and board-declared columns plus aggregate fields."
    else
        out[#out + 1] = "Overall: Current measured dialects satisfy the S0 bar for Q1/Q4/Q5; keep EAV pushdown as the first implementation path and retain this bench as the regression gate."
    end
    return table.concat(out, "\n")
end

local function query_forms_block()
    return [[
## S8a Query Forms

```lua
query_records(db, crm_id, {
  kind = "table",
  object_type = "person",
  filters = {
    { attr = "status", op = "in", value = { "Lead", "Qualified" } },
    { attr = "owner", op = "eq", value = "john" },
    { attr = "score", op = "gte", value = 70 },
    { attr = "last_touch_at", op = "lt", value = "<cutoff>" },
  },
  sort = {
    { attr = "company_name", dir = "asc" },
    { attr = "score", dir = "desc" },
  },
  page = { limit = 50, offset = 5000 },
})

query_records(db, crm_id, {
  kind = "search",
  object_type = "person",
  search = { attr = "name", op = "contains", value = "love" },
  sort = { { attr = "name", dir = "asc" } },
  page = { limit = 50, offset = 100 },
})

query_records(db, crm_id, {
  kind = "board",
  object_type = "opportunity",
  board = {
    collection_id = "sales_pipeline",
    card_attrs = { "name", "company", "amount", "close_date" },
    per_stage_page = { limit = 25 },
  },
  filters = {
    { attr = "owner", op = "eq", value = "john" },
    { attr = "vertical", op = "eq", value = "kickside" },
  },
  sort = { { attr = "score", dir = "desc" } },
})

query_records(db, crm_id, {
  kind = "aggregate",
  object_type = "opportunity",
  aggregate = {
    group_by_stage = { collection_id = "sales_pipeline" },
    agg = {
      { fn = "count" },
      { fn = "sum", attr = "amount" },
    },
  },
  filters = {
    { attr = "owner", op = "eq", value = "john" },
    { attr = "vertical", op = "eq", value = "kickside" },
  },
})
```
]]
end

local function render_results(dialect, seed_stats, rows)
    local generated_at = time.now():utc():format_rfc3339()
    local out = {
        "# S0 Value Store Bench Results",
        "",
        "- Generated at: `" .. generated_at .. "`",
        "- Current dialect: `" .. dialect .. "`",
        "- Fixture: `" .. tostring(seed_stats.people) .. "` person records x 12 attrs; `" .. tostring(seed_stats.opportunities) .. "` opportunity records in a 5-stage pipeline.",
        "- Seed path: direct `spiralscout_crm_record_value_index`, `spiralscout_crm_collection_membership`, and `spiralscout_crm_reconciliation` fixture loading, allowed by the S0 plan for the bench itself.",
        "- Seed time: `" .. fmt_ms(seed_stats.seed_ms) .. " ms`",
        "- Bars: sqlite p95 < " .. SQLITE_BAR_MS .. "ms with no temp B-tree spill; postgres p95 < " .. POSTGRES_BAR_MS .. "ms.",
        "",
        "## Results",
        "",
        result_table(rows),
        "",
        decision_block(rows),
        "",
        "## SQLite Plans",
        "",
    }
    for _, row in ipairs(rows) do
        if row.dialect == "sqlite" then
            out[#out + 1] = "- " .. row.query .. ": " .. markdown_escape(row.sqlite_plan)
        end
    end
    out[#out + 1] = ""
    out[#out + 1] = "## Caveats"
    out[#out + 1] = ""
    out[#out + 1] = "- This benchmark measures SQL pushdown over the existing EAV read-model tables. It does not use production reader pagination because those APIs do not yet expose the S0 query forms."
    out[#out + 1] = "- The fixture is deterministic synthetic GTM-like data; it is intended for storage-shape acceptance, not business forecasting."
    out[#out + 1] = "- Postgres rows are `PENDING` unless the harness is run with a reachable Postgres-backed `app:db` using the repo's `KICKSIDE_PG_*` convention."
    out[#out + 1] = ""
    out[#out + 1] = query_forms_block()
    return table.concat(out, "\n")
end

local function write_results(markdown)
    local vol, err = fs.get("app:workspace_fs")
    check_err(err, "open workspace fs")
    if not vol then
        fail("app:workspace_fs unavailable")
    end
    local exists = false
    local ok, stat_or_err = pcall(function()
        return vol:exists("/bench")
    end)
    if ok then
        exists = stat_or_err == true
    end
    if not exists then
        local _, mkdir_err = vol:mkdir("/bench")
        if mkdir_err then
            local exists_after = false
            pcall(function()
                exists_after = vol:exists("/bench") == true
            end)
            if not exists_after then
                fail("create bench directory: " .. tostring(mkdir_err))
            end
        end
    end
    local written, write_err = vol:writefile(RESULTS_PATH, markdown)
    if not written then
        fail("write " .. RESULTS_PATH .. ": " .. tostring(write_err))
    end
end

function M.load(input)
    local db = open_db()
    local ok, err = pcall(function()
        local loops = tonumber(input and input.loops) or 1
        for _ = 1, loops do
            run_q1(db)
        end
    end)
    db:release()
    if not ok then
        return { ok = false, error = tostring(err) }
    end
    return { ok = true }
end

local function define_tests()
    test.describe("CRM S0 value_store benchmark", function()
        test.it("runs storage acceptance queries and writes bench/RESULTS.md", function()
            wait_for_tables()
            local db = open_db()
            local dialect = db_dialect(db)
            local seed_stats
            local rows
            local ok, err = pcall(function()
                seed_stats = seed_fixture(db)
                rows = benchmark_current_db(db, dialect)
                if dialect == "sqlite" then
                    for _, row in ipairs(pending_rows("postgres")) do
                        rows[#rows + 1] = row
                    end
                elseif dialect == "postgres" then
                    for _, row in ipairs(pending_rows("sqlite")) do
                        rows[#rows + 1] = row
                    end
                end
                write_results(render_results(dialect, seed_stats, rows))
            end)
            db:release()
            if not ok then
                fail(err)
            end
        end)
    end)
end

local run_cases = test.run_cases(define_tests)

function M.run(options)
    return run_cases(options)
end

M.FAST_TEST_FILTERS = FAST_TEST_FILTERS

return M

