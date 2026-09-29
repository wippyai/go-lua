type MergeDecisionRow = { decision: string }
type UnknownMap = { [string]: unknown }
type Command = { type: string, body: UnknownMap }
local types = { EVENTS = { MERGE_DECIDED = "merge_decided" } }
local function open_db(): (any?, string?) return { release = function() end }, nil end
local function sorted_pair(_a: unknown, _b: unknown): (string, string) return "a", "b" end
local function merge_decision_row(_db: any, _crm_id: string, _a: string, _b: string): (MergeDecisionRow?, string?)
    if _crm_id == "" then return nil, nil end
    return { decision = "do_not_merge" }, nil
end
local function merge_decision_ref(_row: MergeDecisionRow, _a: string, _b: string): string return "ref" end
local function actor_id_for_context(): string return "actor" end
local function trim(_v: unknown): string return "reason" end

local function needs_merge_integrity(commands: { Command }): boolean
    for _, cmd in ipairs(commands) do
        local etype = cmd.type
        if etype == types.EVENTS.MERGE_DECIDED then return true end
    end
    return false
end

local function check_merge_integrity(crm_id: string, commands: { Command }): string?
    if not needs_merge_integrity(commands) then return nil end
    local db, derr = open_db()
    if not db then return derr end
    for _, cmd in ipairs(commands) do
        local etype = cmd.type
        local body: UnknownMap = type(cmd.body) == "table" and (cmd.body :: UnknownMap) or {}
        if etype == types.EVENTS.MERGE_DECIDED then
            local left_id, right_id = sorted_pair(body.left_id, body.right_id)
            local row, rerr = merge_decision_row(db, crm_id, left_id, right_id)
            if rerr then db:release(); return rerr end
            local decision = tostring(body.decision or "")
            local is_override = body.override == true
            local prior_is_block = row and tostring(row.decision or "") == "do_not_merge"
            if prior_is_block and decision ~= "do_not_merge" then
                local expected = merge_decision_ref(row, left_id, right_id)
                local reviewer = actor_id_for_context()
                if not is_override or reviewer == "" or trim(body.reason) == ""
                    or trim(body.prior_decision_ref) ~= expected then
                    db:release()
                    return "merge override blocked: prior do_not_merge requires override=true with an authenticated reviewer,"
                        .. " reason, and prior_decision_ref=" .. expected
                end
                body.reviewer_actor = reviewer
            elseif is_override then
                db:release()
                return "merge override blocked: override=true requires an existing do_not_merge decision"
            end
        end
    end
    db:release()
    return nil
end

-- Append a batch of { type, body } commands to the CRM thread after validating the
-- event vocabulary and running write-boundary constraints.
-- The dependency key a record answers to, for callers that release writes
-- blocked on it once the record lands.
