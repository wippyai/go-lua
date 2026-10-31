-- Order-to-cash core: pure lifecycle/validation/totals helpers plus SQL
-- data-access for order headers, lines, and transition events. Reads and writes
-- go through the declared order tables (spiralscout_orders / _order_lines /
-- _order_events); the module ships no sample rows, so a fresh install starts
-- empty and fills as orders are created.

local json = require("json")
local sql = require("sql")
local time = require("time")
local uuid = require("uuid")
local config = require("config")
local events = require("events")

local M = {}

local DB = "app:db"

local TRANSITIONS = {
    draft = { submitted = true, cancelled = true },
    submitted = { approved = true, draft = true, cancelled = true },
    approved = { confirmed = true, cancelled = true },
    confirmed = { fulfilled = true, cancelled = true },
    fulfilled = { invoiced = true },
    invoiced = { closed = true },
    closed = {},
    cancelled = {},
}

local APPROVAL = { approved = true, confirmed = true }
local CANCEL = { cancelled = true }

-- Lifecycle presentation, kept here as the single source served to the UI (no
-- hand-mirrored client copy). PROGRESSION is the linear order-to-cash spine used
-- for the lifeline; TERMINAL marks dead-ends; TRANSITION_LABELS name the action
-- to reach a target status; ROLE_OF maps a required action to its role name.
local ALL_STATUSES = { "draft", "submitted", "approved", "confirmed", "fulfilled", "invoiced", "closed", "cancelled" }
local PROGRESSION = { "draft", "submitted", "approved", "confirmed", "fulfilled", "invoiced", "closed" }
local TERMINAL = { closed = true, cancelled = true }
local TRANSITION_LABELS = {
    submitted = "Submit for review",
    approved = "Approve",
    confirmed = "Confirm",
    fulfilled = "Mark fulfilled",
    invoiced = "Invoice",
    closed = "Close",
    cancelled = "Cancel order",
    draft = "Return to draft",
}
-- Stable per-source ordering of outgoing transitions for a deterministic UI.
local TRANSITION_ORDER = { "submitted", "approved", "confirmed", "fulfilled", "invoiced", "closed", "draft", "cancelled" }
local ROLE_OF = { ["orders.transition"] = "editor", ["orders.approve"] = "approver", ["orders.cancel"] = "canceller" }

-- ─── pure helpers ────────────────────────────────────────────────────────

local function push(errors, path, message)
    errors[#errors + 1] = { path = path, message = message }
end

local function present(value)
    return type(value) == "string" and value ~= ""
end

local function is_array(value)
    if type(value) ~= "table" then return false end
    local n = 0
    for k, _ in pairs(value) do
        if type(k) ~= "number" then return false end
        if k > n then n = k end
    end
    for i = 1, n do
        if value[i] == nil then return false end
    end
    return true
end

local function n(value)
    return type(value) == "number" and value or 0
end

local function round2(value)
    return math.floor((value * 100) + 0.5) / 100
end

function M.can_transition(from_status, to_status)
    local next_statuses = TRANSITIONS[tostring(from_status or "")]
    return type(next_statuses) == "table" and next_statuses[tostring(to_status or "")] == true
end

function M.required_action(from_status, to_status)
    if not M.can_transition(from_status, to_status) then return nil end
    if APPROVAL[to_status] then return "orders.approve" end
    if CANCEL[to_status] then return "orders.cancel" end
    return "orders.transition"
end

-- Outgoing transitions from a status as an ordered list of
-- { to, action, label, role }. The graph source of truth; callers layer per-actor
-- gating on top.
function M.available_transitions(from_status)
    local nexts = TRANSITIONS[tostring(from_status or "")] or {}
    local out = {}
    for _, to in ipairs(TRANSITION_ORDER) do
        if nexts[to] then
            local action = M.required_action(from_status, to)
            local role = action and ROLE_OF[action] or nil
            out[#out + 1] = { to = to, action = action, label = TRANSITION_LABELS[to] or to, role = role }
        end
    end
    return out
end

-- Whether a status is terminal (no outgoing transitions).
function M.is_terminal(status)
    return TERMINAL[tostring(status or "")] == true
end

-- The full lifecycle graph, served so the UI renders lifeline and gates from data.
function M.lifecycle()
    local transitions = {}
    for _, from in ipairs(ALL_STATUSES) do
        transitions[from] = M.available_transitions(from)
    end
    local terminal = {}
    for status, _ in pairs(TERMINAL) do terminal[#terminal + 1] = status end
    table.sort(terminal)
    return {
        states = ALL_STATUSES,
        progression = PROGRESSION,
        terminal = terminal,
        transition_labels = TRANSITION_LABELS,
        transitions = transitions,
    }
end

function M.calculate_totals(lines)
    local subtotal, discount, tax = 0, 0, 0
    for _, line in ipairs(lines or {}) do
        local gross = n(line.quantity) * n(line.unit_price)
        local line_discount = n(line.discount)
        local net = math.max(0, gross - line_discount)
        subtotal = subtotal + gross
        discount = discount + line_discount
        tax = tax + (net * n(line.tax_rate))
    end
    return { subtotal = round2(subtotal), discount = round2(discount), tax = round2(tax), total = round2(subtotal - discount + tax) }
end

local function validate_line(errors, line, prefix)
    if not present(line.sku) then push(errors, prefix .. ".sku", "sku is required") end
    if not present(line.name) then push(errors, prefix .. ".name", "name is required") end
    if type(line.quantity) ~= "number" or line.quantity <= 0 then push(errors, prefix .. ".quantity", "quantity must be greater than zero") end
    if type(line.unit_price) ~= "number" or line.unit_price < 0 then push(errors, prefix .. ".unit_price", "unit_price must be non-negative") end
end

-- Full submit-ready validation: header, currency, and at least one valid line.
function M.validate_order(order)
    local errors = {}
    if type(order) ~= "table" then return false, { { path = "order", message = "order must be an object" } } end
    if not present(order.order_number) then push(errors, "order_number", "order_number is required") end
    if not present(order.customer_name) then push(errors, "customer_name", "customer_name is required") end
    if not present(order.currency) then push(errors, "currency", "currency is required") end
    if not is_array(order.lines) or #order.lines == 0 then
        push(errors, "lines", "at least one order line is required")
    else
        for i, line in ipairs(order.lines) do
            validate_line(errors, line, "lines[" .. tostring(i) .. "]")
        end
    end
    return #errors == 0, errors
end

function M.enrich_order(order)
    local copy = {}
    for k, v in pairs(order or {}) do copy[k] = v end
    copy.status = copy.status or "draft"
    copy.currency = copy.currency or "USD"
    copy.lines = copy.lines or {}
    copy.totals = M.calculate_totals(copy.lines)
    return copy
end

-- Draft-creation validation and shaping. A new order needs only a customer to
-- exist; lines and submission checks come later. Returns (order, errors); order
-- carries no order_number, which the persistence layer assigns.
function M.build_new_order(body)
    body = type(body) == "table" and body or {}
    local errors = {}
    if not present(body.customer_name) then push(errors, "customer_name", "customer_name is required") end

    local currency = present(body.currency) and string.upper(body.currency) or config.default_currency()
    if not config.currency_allowed(currency) then
        push(errors, "currency", currency .. " is not an allowed currency")
    end

    local lines = {}
    if body.lines ~= nil then
        if not is_array(body.lines) then
            push(errors, "lines", "lines must be an array")
        else
            for i, line in ipairs(body.lines) do
                validate_line(errors, line, "lines[" .. tostring(i) .. "]")
                lines[i] = line
            end
        end
    end
    if #errors > 0 then return nil, errors end

    return M.enrich_order({
        customer_name = body.customer_name,
        channel = present(body.channel) and body.channel or nil,
        currency = currency,
        due_date = present(body.due_date) and body.due_date or nil,
        status = "draft",
        lines = lines,
    }), {}
end

function M.format_order(order)
    if not order then return "_Order not found._" end
    local total = (order.totals or {}).total or 0
    local lines = { "# " .. order.order_number .. " - " .. order.customer_name, "", "- Status: " .. order.status, "- Channel: " .. tostring(order.channel or ""), "- Total: " .. tostring(order.currency or "USD") .. " " .. string.format("%.2f", total), "", "## Lines" }
    for _, line in ipairs(order.lines or {}) do lines[#lines + 1] = "- " .. line.sku .. " x" .. tostring(line.quantity) .. " - " .. line.name end
    if #(order.lines or {}) == 0 then lines[#lines + 1] = "_No lines yet._" end
    return table.concat(lines, "\n")
end

function M.format_order_list(orders)
    local lines = { "# Orders (" .. tostring(#(orders or {})) .. ")" }
    for _, order in ipairs(orders or {}) do
        lines[#lines + 1] = "- " .. tostring(order.order_number or "(draft)") .. " [" .. tostring(order.status or "draft") .. "] " .. tostring(order.customer_name or "") .. " - " .. (order.currency or "USD") .. " " .. string.format("%.2f", (order.totals or {}).total or 0)
    end
    if #lines == 1 then lines[#lines + 1] = "_No matching orders._" end
    return table.concat(lines, "\n")
end

-- ─── SQL data-access ─────────────────────────────────────────────────────

local function now()
    return time.now():utc():format_rfc3339()
end

-- Shared body for order lifecycle/line thread events; extra overlays event-specific keys.
local function order_event_body(order, actor_id, extra)
    local body = {
        order_id = order.id,
        order_number = order.order_number,
        customer_name = order.customer_name,
        status = order.status,
        currency = order.currency,
        total = (order.totals or {}).total or 0,
        actor = actor_id,
        occurred_at = now(),
    }
    for k, v in pairs(extra or {}) do body[k] = v end
    return body
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

local function num(v)
    return tonumber(v) or 0
end

local function open()
    local db, err = sql.get(DB)
    if err or not db then return nil, "orders database unavailable: " .. tostring(err) end
    return db, nil
end

local function query(h: any, q: string, params: any): (any, string?)
    local rows, err = h:query(q, params or {})
    if err then return nil, tostring(err) end
    return rows or {}, nil
end

local function exec(h: any, q: string, params: any): string?
    local _, err = h:execute(q, params or {})
    if err then return tostring(err) end
    return nil
end

local function map_line(r)
    return {
        sku = tostring(r.sku or ""),
        name = tostring(r.name or ""),
        quantity = num(r.quantity),
        unit_price = num(r.unit_price),
        discount = num(r.discount),
        tax_rate = num(r.tax_rate),
        line_number = tonumber(r.line_number) or 0,
    }
end

local function map_order(r, lines)
    local snapshot = decode(r.customer_snapshot) or {}
    return {
        id = tostring(r.order_id or ""),
        order_number = tostring(r.order_number or ""),
        customer_name = tostring(snapshot.name or r.customer_ref or ""),
        status = tostring(r.status or "draft"),
        channel = r.channel_code,
        currency = tostring(r.currency or "USD"),
        due_date = r.due_date,
        approval_state = r.approval_state,
        fulfillment_state = r.fulfillment_state,
        invoice_state = r.invoice_state,
        version = tonumber(r.version) or 1,
        created_at = r.created_at,
        updated_at = r.updated_at,
        lines = lines or {},
        totals = M.calculate_totals(lines or {}),
    }
end

-- Queue-row shape: no per-order line hydration (avoids N+1); totals come from the
-- persisted header columns kept in sync on every line mutation, line_count from a
-- single grouped query.
local function map_order_summary(r, line_count)
    local snapshot = decode(r.customer_snapshot) or {}
    return {
        id = tostring(r.order_id or ""),
        order_number = tostring(r.order_number or ""),
        customer_name = tostring(snapshot.name or r.customer_ref or ""),
        status = tostring(r.status or "draft"),
        channel = r.channel_code,
        currency = tostring(r.currency or "USD"),
        due_date = r.due_date,
        approval_state = r.approval_state,
        fulfillment_state = r.fulfillment_state,
        invoice_state = r.invoice_state,
        version = tonumber(r.version) or 1,
        created_at = r.created_at,
        updated_at = r.updated_at,
        line_count = tonumber(line_count) or 0,
        totals = {
            subtotal = num(r.subtotal),
            discount = num(r.discount_total),
            tax = num(r.tax_total),
            total = num(r.grand_total),
        },
    }
end

local function load_lines(db, order_id)
    local rows = query(db, "SELECT * FROM spiralscout_order_lines WHERE order_id = $1 ORDER BY line_number ASC", { order_id })
    local out = {}
    for _, r in ipairs(rows or {}) do out[#out + 1] = map_line(r) end
    return out
end

-- Line counts for a page of order ids in one grouped query (no N+1).
local function line_counts(db, ids)
    local counts = {}
    if #ids == 0 then return counts end
    local marks, params = {}, {}
    for _, id in ipairs(ids) do
        params[#params + 1] = id
        marks[#marks + 1] = "$" .. #params
    end
    local rows = query(db, "SELECT order_id, COUNT(*) AS n FROM spiralscout_order_lines WHERE order_id IN (" .. table.concat(marks, ", ") .. ") GROUP BY order_id", params)
    for _, r in ipairs(rows or {}) do counts[tostring(r.order_id or "")] = tonumber(r.n) or 0 end
    return counts
end

-- The q-search clause (order_number / customer / channel) as a single condition
-- appended to params. Returns "" when no query is given.
local function q_clause(q, params)
    if not present(q) then return "" end
    params[#params + 1] = "%" .. string.lower(tostring(q)) .. "%"
    local i = "$" .. #params
    return "(LOWER(order_number) LIKE " .. i .. " OR LOWER(COALESCE(customer_ref, '')) LIKE " .. i .. " OR LOWER(COALESCE(channel_code, '')) LIKE " .. i .. ")"
end

-- Returns (rows, err, total, status_counts). rows are queue summaries; total is the
-- full match count for the active (q + status) filter, driving pagination.
-- status_counts is keyed by status over the q-only scope so every chip carries its
-- own count regardless of the selected scope.
function M.list_orders(args)
    args = type(args) == "table" and args or {}
    local db, err = open()
    if not db then return nil, err end

    -- q-only scope (for the status chips).
    local qparams = {}
    local qc = q_clause(args.q, qparams)
    local qwhere = qc ~= "" and (" WHERE " .. qc) or ""

    -- active scope (q + status) for the page and total.
    local params, wheres = {}, {}
    local ac = q_clause(args.q, params)
    if ac ~= "" then wheres[#wheres + 1] = ac end
    if present(args.status) then params[#params + 1] = args.status; wheres[#wheres + 1] = "status = $" .. #params end
    local where = #wheres > 0 and (" WHERE " .. table.concat(wheres, " AND ")) or ""

    local count_rows, cerr = query(db, "SELECT COUNT(*) AS n FROM spiralscout_orders" .. where, params)
    if cerr then db:release(); return nil, cerr end
    local total = count_rows[1] and (tonumber(count_rows[1].n) or 0) or 0

    local status_counts = {}
    local srows = query(db, "SELECT status, COUNT(*) AS n FROM spiralscout_orders" .. qwhere .. " GROUP BY status", qparams)
    for _, r in ipairs(srows or {}) do status_counts[tostring(r.status or "")] = tonumber(r.n) or 0 end

    local limit = math.max(1, math.min(200, tonumber(args.limit) or 25))
    local offset = math.max(0, tonumber(args.offset) or 0)
    local lparams = {}
    for _, p in ipairs(params) do lparams[#lparams + 1] = p end
    lparams[#lparams + 1] = limit
    lparams[#lparams + 1] = offset
    local rows, qerr = query(db, "SELECT * FROM spiralscout_orders" .. where ..
        " ORDER BY created_at DESC, order_number DESC LIMIT $" .. (#lparams - 1) .. " OFFSET $" .. #lparams, lparams)
    if qerr then db:release(); return nil, qerr end

    local ids = {}
    for _, r in ipairs(rows) do ids[#ids + 1] = tostring(r.order_id or "") end
    local counts = line_counts(db, ids)
    local out = {}
    for _, r in ipairs(rows) do out[#out + 1] = map_order_summary(r, counts[tostring(r.order_id or "")]) end
    db:release()
    return out, nil, total, status_counts
end

-- Append-only audit trail for an order: created / transition / line_* events with
-- actor, note, and timestamps, oldest first.
function M.list_events(id)
    local order, err = M.get_order(id)
    if not order then return nil, err or "not found" end
    local db, derr = open()
    if not db then return nil, derr end
    local rows, qerr = query(db, "SELECT event_type, from_status, to_status, actor_ref, payload, created_at FROM spiralscout_order_events WHERE order_id = $1 ORDER BY created_at ASC, seq ASC", { order.id })
    if qerr then db:release(); return nil, qerr end
    local out = {}
    for _, r in ipairs(rows or {}) do
        local payload = decode(r.payload) or {}
        out[#out + 1] = {
            event_type = tostring(r.event_type or ""),
            from_status = r.from_status,
            to_status = r.to_status,
            actor_ref = r.actor_ref,
            note = payload.note,
            payload = payload,
            created_at = r.created_at,
        }
    end
    db:release()
    return out, nil
end

function M.get_order(id)
    if not present(id) then return nil, "not found" end
    local db, err = open()
    if not db then return nil, err end
    local rows, qerr = query(db, "SELECT * FROM spiralscout_orders WHERE order_id = $1 OR order_number = $1 LIMIT 1", { id })
    if qerr then db:release(); return nil, qerr end
    if not rows[1] then db:release(); return nil, "not found" end
    local order = map_order(rows[1], load_lines(db, tostring(rows[1].order_id or "")))
    db:release()
    return order, nil
end

-- Sequential prefixed number from config, retried past any manual collisions.
local function next_order_number(db)
    local scheme = config.numbering()
    local rows = query(db, "SELECT COUNT(*) AS n FROM spiralscout_orders", {})
    local base = (scheme.start - 1) + (rows[1] and (tonumber(rows[1].n) or 0) or 0)
    for i = 1, 50 do
        local candidate = scheme.prefix .. tostring(base + i)
        local taken = query(db, "SELECT 1 FROM spiralscout_orders WHERE order_number = $1 LIMIT 1", { candidate })
        if not taken[1] then return candidate end
    end
    return scheme.prefix .. tostring(base) .. "-" .. uuid.v4():sub(1, 6)
end

-- Per-line total: net (gross minus discount, floored at 0) plus its own tax.
local function line_total(line)
    local net = math.max(0, (num(line.quantity) * num(line.unit_price)) - num(line.discount))
    return round2(net + (net * num(line.tax_rate)))
end

-- Returns (order, err, conflict). conflict is true when a supplied order_number
-- is already taken.
function M.create_order(body, actor_id)
    local shaped, errors = M.build_new_order(body)
    if not shaped then return nil, errors, false end

    local db, err = open()
    if not db then return nil, err, false end

    local supplied = type(body) == "table" and present(body.order_number) and tostring(body.order_number) or nil
    if supplied then
        local taken = query(db, "SELECT 1 FROM spiralscout_orders WHERE order_number = $1 LIMIT 1", { supplied })
        if taken[1] then db:release(); return nil, "an order with this number already exists", true end
    end

    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, "begin failed: " .. tostring(tx_err), false end

    local order_id = uuid.v4()
    local order_number = supplied or next_order_number(tx)
    local ts = now()
    local totals = shaped.totals

    local ierr = exec(tx, [[
        INSERT INTO spiralscout_orders
            (order_id, order_number, customer_ref, customer_snapshot, status, channel_code, currency,
             subtotal, discount_total, tax_total, grand_total, due_date, created_by, updated_by, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
    ]], {
        order_id, order_number, shaped.customer_name, encode({ name = shaped.customer_name }), shaped.status,
        shaped.channel, shaped.currency, totals.subtotal, totals.discount, totals.tax, totals.total,
        shaped.due_date, actor_id, actor_id, ts, ts,
    })
    if ierr then tx:rollback(); db:release(); return nil, ierr, false end

    for i, line in ipairs(shaped.lines) do
        local lerr = exec(tx, [[
            INSERT INTO spiralscout_order_lines
                (line_id, order_id, line_number, sku, name, quantity, unit_price, discount, tax_rate, line_total)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        ]], { uuid.v4(), order_id, i, line.sku, line.name, num(line.quantity), num(line.unit_price), num(line.discount), num(line.tax_rate), line_total(line) })
        if lerr then tx:rollback(); db:release(); return nil, lerr, false end
    end

    local everr = exec(tx, [[
        INSERT INTO spiralscout_order_events (event_id, order_id, event_type, to_status, actor_ref, payload)
        VALUES ($1, $2, $3, $4, $5, $6)
    ]], { uuid.v4(), order_id, "created", shaped.status, actor_id, encode({ order_number = order_number }) })
    if everr then tx:rollback(); db:release(); return nil, everr, false end

    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, "commit failed: " .. tostring(commit_err), false end
    local order, gerr = M.get_order(order_id)
    if order then
        events.emit(events.EVENT.CREATED, order_event_body(order, actor_id, { order_number = order_number }),
            "order:" .. order_id .. ":created")
    end
    return order, gerr
end

-- ─── line editing (draft-only) ───────────────────────────────────────────

-- Recompute and persist header totals from the current line set and bump the
-- optimistic-concurrency version, inside an open transaction.
local function persist_header(tx, order_id, actor_id, ts)
    local rows = query(tx, "SELECT * FROM spiralscout_order_lines WHERE order_id = $1 ORDER BY line_number ASC", { order_id })
    local lines = {}
    for _, r in ipairs(rows or {}) do lines[#lines + 1] = map_line(r) end
    local totals = M.calculate_totals(lines)
    return exec(tx, [[
        UPDATE spiralscout_orders
        SET subtotal = $1, discount_total = $2, tax_total = $3, grand_total = $4,
            updated_at = $5, updated_by = $6, version = version + 1
        WHERE order_id = $7
    ]], { totals.subtotal, totals.discount, totals.tax, totals.total, ts, actor_id, order_id })
end

local function line_event(tx, order, event_type, actor_id, payload)
    return exec(tx, [[
        INSERT INTO spiralscout_order_events (event_id, order_id, event_type, from_status, to_status, actor_ref, payload)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    ]], { uuid.v4(), order.id, event_type, order.status, order.status, actor_id, encode(payload) })
end

-- Returns (order, err, conflict). conflict is true when the order is not a draft
-- (lines are frozen once submitted); a validation failure returns err as the
-- structured error list with conflict false.
function M.add_line(id, line, actor_id)
    local order, err = M.get_order(id)
    if not order then return nil, err or "not found", false end
    if order.status ~= "draft" then return nil, "order is not a draft", true end
    local verrs = {}
    validate_line(verrs, type(line) == "table" and line or {}, "line")
    if #verrs > 0 then return nil, verrs, false end

    local db, derr = open()
    if not db then return nil, derr, false end
    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, "begin failed: " .. tostring(tx_err), false end

    local nrows = query(tx, "SELECT COALESCE(MAX(line_number), 0) AS m FROM spiralscout_order_lines WHERE order_id = $1", { order.id })
    local next_no = (nrows[1] and (tonumber(nrows[1].m) or 0) or 0) + 1
    local ts = now()
    local lerr = exec(tx, [[
        INSERT INTO spiralscout_order_lines
            (line_id, order_id, line_number, sku, name, quantity, unit_price, discount, tax_rate, line_total)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    ]], { uuid.v4(), order.id, next_no, line.sku, line.name, num(line.quantity), num(line.unit_price), num(line.discount), num(line.tax_rate), line_total(line) })
    if lerr then tx:rollback(); db:release(); return nil, lerr, false end
    local herr = persist_header(tx, order.id, actor_id, ts)
    if herr then tx:rollback(); db:release(); return nil, herr, false end
    local everr = line_event(tx, order, "line_added", actor_id, { sku = line.sku, line_number = next_no })
    if everr then tx:rollback(); db:release(); return nil, everr, false end

    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, "commit failed: " .. tostring(commit_err), false end
    local updated, gerr = M.get_order(order.id)
    if updated then
        events.emit(events.EVENT.LINE_ADDED,
            order_event_body(updated, actor_id, { line_number = next_no, sku = line.sku }),
            "order:" .. order.id .. ":v" .. tostring(updated.version) .. ":line_added")
    end
    return updated, gerr
end

function M.update_line(id, line_number, patch, actor_id)
    local order, err = M.get_order(id)
    if not order then return nil, err or "not found", false end
    if order.status ~= "draft" then return nil, "order is not a draft", true end
    line_number = tonumber(line_number)
    if not line_number then return nil, "line_number is required", false end

    local existing
    for _, l in ipairs(order.lines) do if l.line_number == line_number then existing = l end end
    if not existing then return nil, "line not found", true end
    patch = type(patch) == "table" and patch or {}
    local merged = {
        sku = present(patch.sku) and patch.sku or existing.sku,
        name = present(patch.name) and patch.name or existing.name,
        quantity = patch.quantity ~= nil and num(patch.quantity) or existing.quantity,
        unit_price = patch.unit_price ~= nil and num(patch.unit_price) or existing.unit_price,
        discount = patch.discount ~= nil and num(patch.discount) or existing.discount,
        tax_rate = patch.tax_rate ~= nil and num(patch.tax_rate) or existing.tax_rate,
    }
    local verrs = {}
    validate_line(verrs, merged, "line")
    if #verrs > 0 then return nil, verrs, false end

    local db, derr = open()
    if not db then return nil, derr, false end
    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, "begin failed: " .. tostring(tx_err), false end
    local ts = now()
    local uerr = exec(tx, [[
        UPDATE spiralscout_order_lines
        SET sku = $1, name = $2, quantity = $3, unit_price = $4, discount = $5, tax_rate = $6, line_total = $7
        WHERE order_id = $8 AND line_number = $9
    ]], { merged.sku, merged.name, merged.quantity, merged.unit_price, merged.discount, merged.tax_rate, line_total(merged), order.id, line_number })
    if uerr then tx:rollback(); db:release(); return nil, uerr, false end
    local herr = persist_header(tx, order.id, actor_id, ts)
    if herr then tx:rollback(); db:release(); return nil, herr, false end
    local everr = line_event(tx, order, "line_updated", actor_id, { sku = merged.sku, line_number = line_number })
    if everr then tx:rollback(); db:release(); return nil, everr, false end

    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, "commit failed: " .. tostring(commit_err), false end
    local updated, gerr = M.get_order(order.id)
    if updated then
        events.emit(events.EVENT.LINE_UPDATED,
            order_event_body(updated, actor_id, { line_number = line_number, sku = merged.sku }),
            "order:" .. order.id .. ":v" .. tostring(updated.version) .. ":line_updated")
    end
    return updated, gerr
end

function M.remove_line(id, line_number, actor_id)
    local order, err = M.get_order(id)
    if not order then return nil, err or "not found", false end
    if order.status ~= "draft" then return nil, "order is not a draft", true end
    line_number = tonumber(line_number)
    if not line_number then return nil, "line_number is required", false end
    local found = false
    for _, l in ipairs(order.lines) do if l.line_number == line_number then found = true end end
    if not found then return nil, "line not found", true end

    local db, derr = open()
    if not db then return nil, derr, false end
    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, "begin failed: " .. tostring(tx_err), false end
    local ts = now()
    local delerr = exec(tx, "DELETE FROM spiralscout_order_lines WHERE order_id = $1 AND line_number = $2", { order.id, line_number })
    if delerr then tx:rollback(); db:release(); return nil, delerr, false end

    -- Resequence remaining lines to 1..n (ascending is collision-free: numbers
    -- only ever move down into freed slots).
    local rows = query(tx, "SELECT line_id, line_number FROM spiralscout_order_lines WHERE order_id = $1 ORDER BY line_number ASC", { order.id })
    local seq = 0
    for _, r in ipairs(rows or {}) do
        seq = seq + 1
        if (tonumber(r.line_number) or 0) ~= seq then
            local rerr = exec(tx, "UPDATE spiralscout_order_lines SET line_number = $1 WHERE line_id = $2", { seq, r.line_id })
            if rerr then tx:rollback(); db:release(); return nil, rerr, false end
        end
    end

    local herr = persist_header(tx, order.id, actor_id, ts)
    if herr then tx:rollback(); db:release(); return nil, herr, false end
    local everr = line_event(tx, order, "line_removed", actor_id, { line_number = line_number })
    if everr then tx:rollback(); db:release(); return nil, everr, false end

    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, "commit failed: " .. tostring(commit_err), false end
    local updated, gerr = M.get_order(order.id)
    if updated then
        events.emit(events.EVENT.LINE_REMOVED,
            order_event_body(updated, actor_id, { line_number = line_number }),
            "order:" .. order.id .. ":v" .. tostring(updated.version) .. ":line_removed")
    end
    return updated, gerr
end

-- ─── lifecycle transition ─────────────────────────────────────────────────

local function derived_states(to_status)
    local approval, fulfillment, invoice = nil, nil, nil
    if to_status == "approved" or to_status == "confirmed" then approval = "approved" end
    if to_status == "fulfilled" then fulfillment = "complete" end
    if to_status == "invoiced" then invoice = "created" end
    return approval, fulfillment, invoice
end

-- Returns (order, err, conflict, invalid). conflict is true when the transition is
-- not allowed from the current status or an optimistic-lock check fails. invalid is
-- the structured error list for a submit/cancel precondition failure (err carries a
-- summary message); conflict is false in that case.
function M.transition_order(id, to_status, note, actor_id, expected_version)
    local order, err = M.get_order(id)
    if not order then return nil, err or "not found", false end
    to_status = tostring(to_status or "")
    if not M.can_transition(order.status, to_status) then return nil, "invalid transition", true end

    if expected_version ~= nil then
        local want = tonumber(expected_version)
        if want and want ~= order.version then
            return nil, "the order changed since you loaded it", true
        end
    end

    if to_status == "cancelled" and not present(note) then
        return nil, "a cancellation note is required", false, { { path = "note", message = "a cancellation note is required" } }
    end

    if to_status == "submitted" then
        local ok, verrs = M.validate_order(order)
        if not ok then return nil, "order is not ready to submit", false, verrs end
    end

    local db, derr = open()
    if not db then return nil, derr, false end
    local tx, tx_err = db:begin()
    if tx_err then db:release(); return nil, "begin failed: " .. tostring(tx_err), false end

    local ts = now()
    local approval, fulfillment, invoice = derived_states(to_status)
    local sets = { "status = $1", "updated_at = $2", "updated_by = $3", "version = version + 1" }
    local params = { to_status, ts, actor_id }
    if approval then params[#params + 1] = approval; sets[#sets + 1] = "approval_state = $" .. #params end
    if fulfillment then params[#params + 1] = fulfillment; sets[#sets + 1] = "fulfillment_state = $" .. #params end
    if invoice then params[#params + 1] = invoice; sets[#sets + 1] = "invoice_state = $" .. #params end
    params[#params + 1] = order.id
    local uerr = exec(tx, "UPDATE spiralscout_orders SET " .. table.concat(sets, ", ") .. " WHERE order_id = $" .. #params, params)
    if uerr then tx:rollback(); db:release(); return nil, uerr, false end

    local everr = exec(tx, [[
        INSERT INTO spiralscout_order_events (event_id, order_id, event_type, from_status, to_status, actor_ref, payload)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    ]], { uuid.v4(), order.id, "transition", order.status, to_status, actor_id, encode({ note = note }) })
    if everr then tx:rollback(); db:release(); return nil, everr, false end

    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return nil, "commit failed: " .. tostring(commit_err), false end
    local updated, gerr = M.get_order(order.id)
    local etype = events.status_event(to_status)
    if updated and etype then
        events.emit(etype, order_event_body(updated, actor_id, { from_status = order.status, note = note }),
            "order:" .. order.id .. ":" .. to_status)
    end
    return updated, gerr
end

return M
