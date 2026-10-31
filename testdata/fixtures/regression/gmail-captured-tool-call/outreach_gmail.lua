-- The Gmail leg. It resolves a deal's primary contact to the address a draft is
-- sent to, and it creates the draft through the connector's PUBLISHED write
-- tool, addressed at runtime -- the same composition rule the CRM legs follow,
-- so the connector's absence is a reported state rather than a broken import,
-- and no consumer of this module carries the connector's stack. It creates a
-- DRAFT and only ever a draft: the message lands in the sender's drafts folder,
-- and a person opens it and decides whether it sends. Nothing here calls send.
local funcs = require("funcs")
local json = require("json")
local registry = require("registry")
local consts = require("consts")

local M = {}

-- Seams: the plain call (contact resolution) and the connection-scoped call
-- (the write tool), so a suite drives both legs without a connector.
M._call = nil :: any
M._tool_call = nil :: any

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return ((value :: string):gsub("^%s*(.-)%s*$", "%1"))
end

local function available(): boolean
    local entry, err = registry.get(consts.GMAIL_WRITE_TOOL)
    return not err and type(entry) == "table"
end
M._available = available

-- The email a draft is addressed to: the first contact linked on the deal,
-- resolved to its address. A deal with no linked contact or no address on it
-- yields no recipient, and the draft is created without a To for a person to
-- fill -- better a draft they address than no draft at all.
function M.recipient(crm_id: string, deal: any): (string, string)
    local contacts = deal.contacts or deal.contact
    local ref_id = ""
    if type(contacts) == "table" then
        for _, ref in ipairs(contacts :: any) do
            if type(ref) == "table" and trim((ref :: any).id) ~= "" then ref_id = trim((ref :: any).id); break end
        end
    end
    if ref_id == "" then return "", "" end
    local call = M._call or funcs.call
    local raw, err = call(consts.CRM_GET_RECORD, { crm_id = crm_id, record_id = ref_id })
    if err then return "", "" end
    local decoded = json.decode(tostring(raw))
    local envelope: any = decoded
    if type(envelope) ~= "table" or envelope.ok ~= true then return "", "" end
    local values: any = type(envelope.data) == "table" and (envelope.data.values or {}) or {}
    local email = trim(values.email or values.contact_email or values.work_email)
    local name = trim(values.name or values.full_name or values.first_name)
    return email, name
end

-- The tool answers prose -- "Draft created: id <id>" or "Error: <why>" -- and
-- that prefix is its contract. It is parsed strictly: an answer that is neither
-- is a failure that carries the answer, so a wording change upstream surfaces
-- loudly on the card instead of passing as silently created.
local CREATED_PREFIX = "Draft created: id "

function M.parse_created(answer: any): (string?, string?)
    local text = trim(answer)
    if text:sub(1, #CREATED_PREFIX) == CREATED_PREFIX then
        local id = trim(text:sub(#CREATED_PREFIX + 1))
        if id ~= "" then return id, nil end
        return nil, "Gmail created the draft but returned no id"
    end
    if text:sub(1, 7) == "Error: " then return nil, text:sub(8) end
    return nil, "the Gmail tool answered something unexpected: " .. text:sub(1, 120)
end

-- create(connection_id, to, subject, body) -> (draft_id, err). One draft in the
-- connected mailbox, through the write tool under the named connection. An
-- absent connector or an empty connection id is a caller error the generator
-- reports on the card, never a silent no-op.
function M.create(connection_id: string, to: string, subject: string, body: string): (string?, string?)
    if trim(connection_id) == "" then return nil, "no Gmail connection is configured for outreach" end
    if not (M._available or available)() then return nil, "the Gmail connector is not installed" end
    local args: any = { action = "create_draft", subject = subject, body = body }
    if trim(to) ~= "" then args.to = to end
    local tool_call = M._tool_call or function(id: string, input: any, context: any): (any, any)
        return funcs.new():with_context(context):call(id, input)
    end
    local answer, err = tool_call(consts.GMAIL_WRITE_TOOL, args, { connection_id = trim(connection_id) })
    if err then return nil, "the Gmail draft could not be created: " .. tostring(err) end
    local id, parse_err = M.parse_created(answer)
    if not id then return nil, "the Gmail draft could not be created: " .. tostring(parse_err) end
    return id, nil
end

-- The address a reviewer opens the draft at. Gmail addresses a draft by its
-- own id in the drafts view.
function M.link(draft_id: string): string
    local id = trim(draft_id)
    if id == "" then return "https://mail.google.com/mail/u/0/#drafts" end
    return "https://mail.google.com/mail/u/0/#draft=" .. id
end

return M
