-- The HubSpot v4 association batch read, and the paging discipline every caller
-- of it shares.
--
-- One from-object's association list is a PAGED answer: the batch response
-- carries a `paging.next.after` per input, and a reader that ignores it silently
-- truncates every high-fanout object. Both readers here -- the association
-- source that emits one item per edge, and the engagement hydration that folds
-- the associated ids onto the pulled item -- drain those continuations the same
-- way, so the discipline lives in one place rather than twice.
local types = require("types")

local M = {}

-- HubSpot answers a batch read for many objects at once; the source's control
-- bounds cap a page at 100 objects, so one round of hydration costs one request
-- per 100 objects per associated type.
M.MAX_BATCH_INPUTS = 100

-- A from-object whose association list never stops paging is a data fault. The
-- bound makes it visible as a failed page instead of a truncated answer.
M.MAX_PAGES = 100

type ErrorResult = {
    success: boolean,
    error: string?,
    status_code: integer?,
    retry_after_ms: number?,
}
type Transport = {
    batch_read_associations: fun(
        conn: types.Conn,
        from_type: string,
        to_type: string,
        inputs: { types.AssociationInput }
    ): types.AssociationBatchResult,
}

local function trim(value: unknown): string
    if type(value) ~= "string" and type(value) ~= "number" then return "" end
    return (tostring(value):gsub("^%s+", ""):gsub("%s+$", ""))
end

-- ignorable_partial_errors(data) -> (every partial error is an object that no
-- longer exists, how many partial errors there were). A 207 naming a deleted
-- object is an answer; anything else is a failure the caller must not read as an
-- empty association set.
function M.ignorable_partial_errors(data: types.AssociationBatchData): (boolean, integer)
    local errors = type(data.errors) == "table" and data.errors or {}
    local count = #errors
    if data.numErrors ~= nil and (
        data.numErrors < 0
        or data.numErrors % 1 ~= 0
        or data.numErrors ~= count
    ) then
        return false, count
    end
    if count == 0 then return true, 0 end
    for _, item in ipairs(errors) do
        if type(item) ~= "table" or type(item.category) ~= "string" then return false, count end
        local category = item.category:upper()
        if category ~= "OBJECT_NOT_FOUND" then return false, count end
    end
    return true, count
end

-- next_after(paging) -> the continuation cursor for ONE input's association
-- page, or nil when that input is drained.
function M.next_after(paging: types.Paging?): string?
    if not paging or not paging.next then return nil end
    local after = paging.next.after
    if type(after) == "string" and after ~= "" then return after end
    return nil
end

local function slice(values: { string }, first: number, last: number): { types.AssociationInput }
    local out: { types.AssociationInput } = {}
    for index = first, math.min(last, #values) do
        out[#out + 1] = { id = values[index] } :: types.AssociationInput
    end
    return out
end

-- read_ids(tp, conn, from_type, to_type, ids) -> ({ [from_id]: { to_id } }, err).
-- The complete associated-id set for each requested object: chunked at
-- MAX_BATCH_INPUTS objects per request, and each chunk drained through its
-- per-input continuations before the next chunk is read. An unread object
-- answers an empty set, and a failed read answers an error -- never a set the
-- caller cannot tell from an empty one.
function M.read_ids(
    tp: Transport,
    conn: types.Conn,
    from_type: string,
    to_type: string,
    ids: { string }
): ({ [string]: { string } }?, ErrorResult?)
    local out: { [string]: { string } } = {}
    local seen: { [string]: { [string]: boolean } } = {}
    for _, id in ipairs(ids) do
        out[id] = {}
        seen[id] = {}
    end
    if #ids == 0 then return out, nil end
    if type(tp.batch_read_associations) ~= "function" then
        return nil, {
            success = false,
            error = "HubSpot association hydration requires batch association read support",
            status_code = 0,
        }
    end

    for start = 1, #ids, M.MAX_BATCH_INPUTS do
        local inputs = slice(ids, start, start + M.MAX_BATCH_INPUTS - 1)
        local page = 0
        while #inputs > 0 do
            page = page + 1
            if page > M.MAX_PAGES then
                return nil, {
                    success = false,
                    error = "HubSpot " .. from_type .. " -> " .. to_type
                        .. " associations exceeded " .. tostring(M.MAX_PAGES) .. " continuation pages",
                    status_code = 0,
                }
            end
            local result = tp.batch_read_associations(conn, from_type, to_type, inputs)
            if type(result) ~= "table" or result.success ~= true then
                return nil, (type(result) == "table" and result :: ErrorResult)
                    or { success = false, error = "HubSpot association batch returned no result", status_code = 0 }
            end
            local data = result.data
            if type(data) ~= "table" or type(data.results) ~= "table" then
                return nil, {
                    success = false,
                    error = "invalid HubSpot association response: missing results",
                    status_code = 0,
                }
            end
            local ignorable, partial_error_count = M.ignorable_partial_errors(data)
            if partial_error_count > 0 and not ignorable then
                return nil, {
                    success = false,
                    status_code = result.status_code,
                    error = "HubSpot association batch returned " .. tostring(partial_error_count) .. " partial errors",
                }
            end
            local continuations: { types.AssociationInput } = {}
            for _, row in ipairs(data.results) do
                local from_id = trim(row.from and row.from.id or nil)
                local bucket = out[from_id]
                if bucket ~= nil then
                    local known = seen[from_id]
                    for _, association in ipairs(type(row.to) == "table" and row.to or {}) do
                        local to_id = trim(association.toObjectId)
                        -- The answer is the SET of associated objects: one
                        -- object reached through several association types is
                        -- still one associated object.
                        if to_id ~= "" and not known[to_id] then
                            known[to_id] = true
                            bucket[#bucket + 1] = to_id
                        end
                    end
                    local after = M.next_after(row.paging)
                    if after then
                        continuations[#continuations + 1] = { id = from_id, after = after } :: types.AssociationInput
                    end
                end
            end
            inputs = continuations
        end
    end
    return out, nil
end

return M

