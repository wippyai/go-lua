-- Fireflies as a record writer: the connector half of the write-back capability.
--
-- The capability asks two things of a connector. What can you change (describe),
-- and change this (update_record). Fireflies answers the first with the truth
-- and nothing more: ONE record kind, ONE field. Its GraphQL API carries
-- updateMeetingTitle and no mutation for speaker names or transcript text, so a
-- writer that claimed those fields would be promising a write that silently does
-- not happen -- the caller would record a durable fact saying a speaker was
-- renamed at the source, and nothing there would ever have changed.
--
-- The connection is resolved under the CALLER's identity: an explicit
-- component_id when the caller opened the binding with one, otherwise the single
-- Fireflies connection in that identity's scope. A push therefore writes through
-- the same credential the calls were pulled with, and an identity that owns no
-- connection cannot write through someone else's.
local ctx = require("ctx")
local transport = require("transport")

local M = {}

local SYSTEM = "fireflies"
local KIND_TRANSCRIPT = "transcript"
local FIELD_TITLE = "title"

type Deps = {
    transport: any?,
    component_id: string?,
}
type Describe = { system: string, kinds: { string }, fields: { string } }
type UpdateResult = {
    success: boolean,
    updated_fields: { string }?,
    error: string?,
    retriable: boolean?,
    status_code: integer?,
}

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return ((value :: string):gsub("^%s*(.-)%s*$", "%1"))
end

-- A refusal on the merits: the request itself is the reason, so sending it again
-- unchanged produces this same answer. Everything this writer rejects before
-- reaching the API is of that kind.
local function refused(message: string): UpdateResult
    return { success = false, error = message, retriable = false } :: UpdateResult
end

-- Whether a caller should try this write again, decided by the status the call
-- answered under and nothing else:
--
--   0     the request never reached Fireflies -- DNS, TLS or a dropped socket.
--   429   rate limited. The write was refused for now, not on its merits.
--   5xx   the service is erroring; the identical write may well land later.
--
-- Everything else is a decision about THIS write, and repeating it changes
-- nothing: a GraphQL-level refusal (Fireflies answers HTTP 200 with an errors
-- array for a non-admin key or a meeting owned outside the team), 401 for a bad
-- key, 403, 404 for a transcript that is not there. Reading the status is what
-- separates the two -- the message alone cannot, because a rate limit and an
-- admin refusal are both free text, and flattening them together turns an outage
-- into a caller's permanent "the source said no".
--
-- The status leads rather than the message shape, so a GraphQL errors array
-- served under 429 is still read as rate limiting.
local function transient(status_code: integer): boolean
    return status_code == 0 or status_code == 429 or (status_code >= 500 and status_code < 600)
end

function M.describe(_args: any, _deps: Deps?): Describe
    return { system = SYSTEM, kinds = { KIND_TRANSCRIPT }, fields = { FIELD_TITLE } } :: Describe
end

-- update_record({ system, kind, external_id, fields }) -> { success, updated_fields, error? }
--
-- Every refusal is an answer rather than a raise: the caller decides whether a
-- rejected write is worth retrying, and it can only decide that if it is told
-- what was rejected. That decision needs more than a sentence, so a failure also
-- says whether it is `retriable` and under which `status_code` it was answered.
-- A rate limit and a non-admin key are both free text from this API and read
-- identically; without the flag a caller has to treat an outage as a permanent
-- refusal, and a rename is then lost to a 429 that would have cleared on its own.
--
-- What is NOT accepted here is as important as what is -- a request naming
-- another system, another record kind, or a field this API cannot write is
-- refused by name instead of being partially carried out.
function M.update_record(args: any, deps: Deps?): UpdateResult
    local request: { [string]: any } = type(args) == "table" and (args :: { [string]: any }) or {}
    local injected: Deps = (type(deps) == "table" and (deps :: Deps) or {}) :: Deps
    local tp = injected.transport or transport

    -- system and kind are REQUIRED by the contract, so a request carrying neither
    -- is refused rather than read as this writer's own defaults. Defaulting them
    -- would answer a request that addressed no connector at all as though it had
    -- addressed this one, which is precisely the caller most in need of being
    -- told it is misaddressed.
    local system = trim(request.system)
    if system == "" then
        return refused("a write names the system it addresses; this writer serves " .. SYSTEM)
    end
    if system ~= SYSTEM then
        return refused("this writer updates " .. SYSTEM .. " records, not " .. system)
    end
    local kind = trim(request.kind)
    if kind == "" then
        return refused("a write names the record kind it updates; Fireflies writes "
            .. KIND_TRANSCRIPT .. "s")
    end
    if kind ~= KIND_TRANSCRIPT then
        return refused("Fireflies has no writable " .. kind .. " record; it writes transcripts")
    end
    local external_id = trim(request.external_id)
    if external_id == "" then
        return refused("a Fireflies write names the transcript it updates")
    end

    local fields: { [string]: any } = type(request.fields) == "table"
        and (request.fields :: { [string]: any }) or {}
    for name in pairs(fields) do
        if tostring(name) ~= FIELD_TITLE then
            -- The whole reason describe exists. A caller that asks for a speaker
            -- name learns it cannot be written, here, rather than being told the
            -- write succeeded because the title part of it did.
            return refused("Fireflies cannot write " .. tostring(name)
                .. "; its API updates the title and nothing else")
        end
    end
    local title = trim(fields[FIELD_TITLE])
    if title == "" then
        return refused("a Fireflies title write needs a title")
    end

    local conn, conn_err = tp.connect(injected.component_id or (ctx.get("component_id") :: string))
    if conn_err or not conn then
        return refused(tostring(conn_err or "no Fireflies connection is available to write through"))
    end

    local result = tp.update_meeting_title(conn, external_id, title)
    if type(result) ~= "table" or (result :: any).success ~= true then
        local answer: { [string]: any } = type(result) == "table" and (result :: any) or {}
        local message = trim(answer.error)
        if message == "" then
            message = type(result) == "table" and "the Fireflies title update failed"
                or "the Fireflies API returned nothing"
        end
        -- A result the transport did not shape at all is read as status 0, which
        -- is the same answer as a request that never left: nothing here says the
        -- write was judged, so the caller is allowed to ask again.
        local status_code = math.floor(tonumber(answer.status_code) or 0)
        return {
            success = false,
            error = message,
            retriable = transient(status_code),
            status_code = status_code,
        } :: UpdateResult
    end

    return { success = true, updated_fields = { FIELD_TITLE } } :: UpdateResult
end

return M
