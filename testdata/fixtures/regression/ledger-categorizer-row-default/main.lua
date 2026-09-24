-- Booking what the categorizer proposed. This is the household's act, not the
-- agent's: every row here travels the ordinary command path — the same
-- categorize, link_transfer and mark_duplicate a person clicks — so a proposal
-- has no privileged route into the book and every refusal a person would meet
-- is met here too. What the ledger would not take comes back named.
local consts: any = nil
local repo: any = nil
local book: any = nil
local types = require("types")

local M = {}

type Row = types.Row

-- What one call may book. Larger than a screen of proposals and small enough
-- that a refusal list is still readable.
M.MAX_ENTRIES = 200

local function trim(v: any): string
    if v == nil then return "" end
    return (tostring(v):gsub("^%s+", ""):gsub("%s+$", ""))
end

-- One proposal booked the way its shape says. A withdrawal comes first: an
-- entry the household is removing as a repeat is not also categorized.
local function apply(ledger_id: string, p: Row): (Row?, string?)
    local entry_id = trim(p.entry_id)
    local duplicate_of = trim(p.duplicate_of)
    if duplicate_of ~= "" then
        local result, err = book.mark_duplicate(ledger_id, entry_id, duplicate_of)
        if err or not result then return nil, tostring(err) end
        return { entry_id = entry_id, removed = true }, nil
    end
    local kind = trim(p.kind)
    if kind == consts.ENTRY_KIND_TRANSFER then
        local counterpart = trim(p.counterpart_entry_id)
        local result, err = book.link_transfer(ledger_id, entry_id, counterpart, {
            actor = "agent", reason = trim(p.reason),
        })
        if err or not result then return nil, tostring(err) end
        return { entry_id = entry_id, counterpart_entry_id = counterpart }, nil
    end
    local input: Row = {
        categorized_by = "agent", confidence = tonumber(p.confidence) or 0, reason = trim(p.reason),
    }
    if kind == consts.ENTRY_KIND_INTERNAL then
        input.kind = consts.ENTRY_KIND_INTERNAL
    else
        input.account_path = trim(p.account_path)
    end
    local result, err = book.categorize(ledger_id, entry_id, input)
    if err or not result then return nil, tostring(err) end
    return { entry_id = entry_id, unlinked_entry_id = (result :: Row).unlinked_entry_id }, nil
end

-- book(ledger_id, opts) -> ({ booked, failed, unlinked_entry_ids }, err).
-- `opts.entry_ids` books exactly those proposals; without it every open proposal
-- at or above `opts.min_confidence` is booked, which is what the queue's
-- "Book all proposals" does.
function M.book(ledger_id: string, opts: Row?): (Row?, string?)
    local o: Row = opts or {}
    local id = tostring(ledger_id or "")
    if id == "" then return nil, "ledger_id is required" end

    local proposals: { Row } = {}
    local named: { any } = (type(o.entry_ids) == "table" and o.entry_ids or {}) :: { any }
    local failed: { Row } = {}
    if #named > 0 then
        if #named > M.MAX_ENTRIES then
            return nil, "one call books at most " .. tostring(M.MAX_ENTRIES) .. " proposals"
        end
        for _, raw in ipairs(named) do
            local entry_id = trim(raw)
            local proposal, err = repo.get_proposal(id, entry_id)
            if err then return nil, tostring(err) end
            if not proposal then
                failed[#failed + 1] = { entry_id = entry_id, error = "this entry carries no proposal" }
            else
                proposals[#proposals + 1] = proposal :: Row
            end
        end
    else
        local floor = tonumber(o.min_confidence) or consts.BULK_PROPOSAL_CONFIDENCE
        local listed, err = repo.list_proposals(id, floor, M.MAX_ENTRIES)
        if err then return nil, tostring(err) end
        proposals = (listed or {}) :: { Row }
    end

    local booked = 0
    local unlinked: { string } = {}
    -- An entry that has already left the queue in this pass — the mirror leg of a
    -- transfer, or an entry withdrawn as a duplicate — is not booked twice.
    local decided: { [string]: boolean } = {}
    for _, p in ipairs(proposals) do
        local entry_id = trim(p.entry_id)
        if decided[entry_id] then
            failed[#failed + 1] = { entry_id = entry_id, error = "this entry was already decided by an earlier proposal in this pass" }
        else
            local result, err = apply(id, p)
            if err or not result then
                failed[#failed + 1] = { entry_id = entry_id, error = tostring(err) }
            else
                booked = booked + 1
                decided[entry_id] = true
                local counterpart = trim((result :: Row).counterpart_entry_id)
                if counterpart ~= "" then decided[counterpart] = true end
                local freed = trim((result :: Row).unlinked_entry_id)
                if freed ~= "" then unlinked[#unlinked + 1] = freed end
            end
        end
    end
    return { booked = booked, failed = failed, unlinked_entry_ids = unlinked }, nil
end

return M
