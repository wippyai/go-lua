local context = require("context")
local queries = require("queries")
local recall = require("recall")
local graph = require("graph")
local support = require("support")

local function copy_table(value: any): any
    local out = {}
    for key, item in pairs(type(value) == "table" and value or {}) do out[key] = item end
    return out
end

local function utf8_prefix(value: string, byte_limit: number): string
    local index = 1
    local last = 0
    while index <= #value do
        local first = value:byte(index) or 0
        local width = first < 0x80 and 1
            or (first < 0xE0 and 2 or (first < 0xF0 and 3 or 4))
        if index + width - 1 > #value then width = 1 end
        if index + width - 1 > byte_limit then break end
        last = index + width - 1
        index = last + 1
    end
    return value:sub(1, last)
end

local function compact_text(value: any, byte_limit: number, one_line: boolean?): (any, boolean)
    if type(value) ~= "string" then return value, false end
    local text = support.trim(value)
    if one_line then text = text:gsub("%s+", " ") end
    if #text <= byte_limit then return text, false end
    return utf8_prefix(text, math.max(1, byte_limit - 3)) .. "...", true
end

local function compact_record(value: any, limits: any): any
    if type(value) ~= "table" then return value end
    local out = copy_table(value)
    for key, rule in pairs(limits or {}) do
        local limit = type(rule) == "table" and tonumber((rule       ).limit) or tonumber(rule)
        local one_line = type(rule) == "table" and (rule       ).one_line == true
        if limit then
            local compact, truncated = compact_text(out[key], limit, one_line)
            out[key] = compact
            if truncated then out[key .. "_truncated"] = true end
        end
    end
    return out
end

local function compact_records(values: any, limits: any): any
    local out: { any } = {}
    for _, value in ipairs(type(values) == "table" and values or {}) do
        out[#out + 1] = compact_record(value, limits)
    end
    return out
end

local function nonempty(value: any): boolean
    return type(value) == "table" and next(value       ) ~= nil
end

-- ReadJournal is paid from the model's context. The projection stays exact,
-- while the default branch lens returns enough of each recent fact to decide
-- what deserves a full drill. `detail=full` is the lossless door over the same
-- task id, so compaction is explicit and reversible rather than silent loss.
local function compact_branch(value: any): any
    if type(value) ~= "table" then return value end
    local branch = copy_table(value)
    local task_limits = {
        title = { limit = 240, one_line = true },
        description = 900,
        result = 900,
        distilled = 900,
        digest = 1200,
    }
    branch.task = compact_record(branch.task, task_limits)
    branch.ancestors = compact_records(branch.ancestors, task_limits)
    branch.children = compact_records(branch.children, task_limits)
    local facts: { any } = {}
    for _, raw in ipairs(type(branch.facts) == "table" and branch.facts or {}) do
        local fact = compact_record(raw, {
            title = { limit = 240, one_line = true },
            body = 1200,
        })
        -- Projection/runtime bookkeeping does not help an agent choose a fact.
        -- Identity, time, validity, source, tags, and references remain.
        fact.embedding_status = nil
        fact.actor_type = nil
        fact.span_id = nil
        if not nonempty(fact.metadata) then fact.metadata = nil end
        for _, key in ipairs({
            "tags", "linked_component_ids", "linked_upload_ids",
            "linked_file_paths", "linked_task_ids",
        }) do
            if not nonempty(fact[key]) then fact[key] = nil end
        end
        if type(fact.tags) == "table" and #fact.tags > 12 then
            local tags: { any } = {}
            for index = 1, 12 do tags[index] = fact.tags[index] end
            fact.tags = tags
            fact.tags_truncated = true
        end
        facts[#facts + 1] = fact
    end
    branch.facts = facts
    branch.facts_detail = "compact"
    branch.fact_body_limit_bytes = 1200
    return branch
end

local function branch_resume_prompt(branch: any): string
    local task = branch and branch.task
    if type(task) ~= "table" then
        return "You are at the task tree root. Open a task branch to see where work stands, "
            .. "record facts under its task_id, and decompose it into subtasks when work splits."
    end
    local titles: { string } = {}
    for _, ancestor in ipairs(branch.ancestors or {}) do
        local title = compact_text((ancestor       ).title, 120, true)
        if title ~= "" then titles[#titles + 1] = title end
    end
    local chain = #titles > 0 and table.concat(titles, " > ") or "(root)"
    local open_children = branch.counts and tonumber((branch.counts       ).open_children) or 0
    if branch.resolved == "active" then
        local title = compact_text((task       ).title or (task       ).id, 180, true)
        return "You are on the active task \"" .. tostring(title) .. "\" under " .. chain
            .. ". It has " .. tostring(open_children) .. " open subtasks. Entries, checkpoints, and artifacts "
            .. "recorded without a task_id land here; record focus on an ancestor to go back up before working "
            .. "elsewhere, and decompose this task into subtasks when the work splits."
    end
    local title = compact_text((task       ).title or (task       ).id, 180, true)
    return "You are on task \"" .. tostring(title) .. "\" under " .. chain
        .. ". It has " .. tostring(open_children) .. " open subtasks. Record facts (entries, checkpoints, "
        .. "artifacts) under this task_id, and decompose it into subtasks when the work splits."
end

local function handler(input: any): any
    local journal_id, id_err = support.journal_id(input)
    if not journal_id then return { success = false, error = id_err } end
    local view = support.trim(input and input.view)
    if view == "" then view = "context" end
    local value = nil
    local err: string? = nil
    if view == "brief" then
        value, err = queries.brief(journal_id, { limit = input.limit })
        if value then value = { brief = value } end
    elseif view == "context" then
        value, err = context.pack(journal_id, { tail_limit = input.limit })
        if value and not err then
            local page, page_err = graph.catalog(journal_id, { limit = 50 })
            if page then
                value.graphs = (page       ).items
                value.graphs_truncated = (page       ).has_more == true
            end
            err = page_err
        end
        if value then value = { context = value } end
    elseif view == "cursor" then
        value, err = context.cursor(journal_id, {
            cursor_id = input.cursor_id,
            agent_id = input.agent or input.agent_id,
            limit = input.limit,
            inherited_limit = input.inherited_limit,
            delta_limit = input.delta_limit,
        })
        if value then value = { context = value } end
    elseif view == "tail" then
        value, err = queries.state(journal_id, { limit = input.limit })
        if value then value = { events = value.events, tasks = value.tasks, graphs = value.graphs } end
    elseif view == "authority" then
        value, err = queries.authority(journal_id, { limit = input.limit })
        if value then value = { authority = value } end
    elseif view == "search" then
        if support.trim(input.search_mode) == "semantic" then
            value, err = recall.search_vector(journal_id, tostring(input.query or ""), {
                limit = input.limit,
                validity = input.validity,
            })
        else
            value, err = queries.search_text(journal_id, tostring(input.query or ""), {
                limit = input.limit,
                validity = input.validity,
            })
        end
        if value then value = { events = value } end
    elseif view == "references" then
        value, err = queries.references(journal_id, {
            graph_id = input.graph_id,
            node_id = input.node_id,
            ref_id = input.ref_id,
            query = input.query,
            include_removed = input.include_removed == true,
            limit = input.limit,
            offset = input.offset,
        })
        if value then value = { references = value } end
    elseif view == "graphs" then
        local page, page_err = graph.catalog(journal_id, {
            query = input.query,
            limit = input.limit,
            after_rank = input.after_rank,
            after_updated_at = input.after_updated_at,
            after_graph_id = input.after_graph_id,
        })
        err = page_err
        if page then
            value = {
                graphs = (page       ).items,
                has_more = (page       ).has_more,
                next_cursor = (page       ).next_cursor,
            }
        end
    elseif view == "tasks" then
        value, err = queries.tasks(journal_id, {
            query = input.query,
            status = input.task_status or "active",
            limit = input.limit,
        })
        if value then value = { tasks = value } end
    elseif view == "branch" then
        local detail = string.lower(support.trim(input.detail))
        if detail == "" then detail = "compact" end
        local branch, branch_err = queries.branch(journal_id, input.task_id, {
            fact_limit = input.limit,
            lease_id = input.lease_id,
            roots = input.roots == true,
        })
        if branch then
            local prompt = branch_resume_prompt(branch)
            if detail ~= "full" then branch = compact_branch(branch)
            else branch.facts_detail = "full" end
            value = { branch = branch, detail = detail, resume_prompt = prompt }
        else err = branch_err end
    elseif view == "frontier" then
        value, err = queries.frontier(journal_id, {
            drill = input.category,
            root_task_id = input.task_id,
            limit = input.limit,
        })
        if value then value = { frontier = value } end
    elseif view == "changes" then
        value, err = queries.changes(journal_id, {
            since_seq = input.since_seq,
            since_checkpoint = input.agent,
            scope = input.scope,
            task_id = input.task_id,
            limit = input.limit,
        })
        if value then value = { changes = value } end
    elseif view == "tree" then
        value, err = queries.tree(journal_id, input.task_id, input.depth, input.limit)
        if value then value = { tree = value } end
    elseif view == "health" then
        value, err = queries.health(journal_id)
        if value then value = { health = value } end
    elseif view == "spans" then
        value, err = queries.spans(journal_id, {
            state = input.span_state,
            agent_id = input.span_agent_id or input.agent,
            task_id = input.task_id,
            since = input.since,
            limit = input.limit,
            after_activity_at = input.after_activity_at,
            after_span_id = input.after_span_id,
        })
        if value then value = { spans = value } end
    elseif view == "activity" then
        value, err = queries.activity(journal_id, {
            bucket = input.bucket,
            agent_id = input.span_agent_id or input.agent,
            since = input.since,
            until_ts = input.until_ts,
            limit = input.limit,
            after_bucket_at = input.after_bucket_at,
            after_agent_id = input.after_agent_id,
        })
        if value then value = { activity = value } end
    else
        err = "unknown Journal read view"
    end
    if err then return { success = false, error = tostring(err), view = view, journal_id = journal_id } end
    local out = { success = true, view = view, journal_id = journal_id }
    for key, item in pairs(value or {}) do out[key] = item end
    return support.maintain_position(journal_id, {
        cursor_id = input and input.cursor_id,
        agent_id = input and input.agent_id,
    }, out)
end

return { handler = handler }
