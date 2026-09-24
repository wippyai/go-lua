-- keeper.knowledge.service:service
--
-- Business layer for the knowledge base. Validates input, resolves the
-- target KB, delegates CRUD to the repo, and owns orchestration around
-- embedding and registry-driven research flows. HTTP handlers stay thin
-- adapters that translate service errors into response bodies.

local sql = require("sql")

local consts = require("kb_consts")
local kb_repo = require("kb_repo")
local embedder = require("embedder")
local llm = require("llm")
local flow = require("flow")

local M = {}

local KNOWLEDGE_CURATOR_AGENT = "keeper.agents:kb_curator"

type EmbedArgs = {
    node_id: string?,
    model: string?,
}

type Embedder = {
    embed: (EmbedArgs) -> (unknown, string?),
}

local embedder_mod = embedder :: Embedder

M.ERR = {
    BAD_REQUEST  = "bad_request",
    NOT_FOUND    = "not_found",
    FORBIDDEN    = "forbidden",
    UNAUTHORIZED = "unauthorized",
    CONFLICT     = "conflict",
    INTERNAL     = "internal",
}

local function fail(code, message, extra)
    local err = { code = code, message = message }
    if extra then
        for k, v in pairs(extra) do err[k] = v end
    end
    return nil, err
end

local function get_state_db()
    return sql.get("keeper.state:db")
end

local function resolve_kb_id(kb_param)
    if not kb_param or kb_param == "" then return nil, nil end
    local kb = kb_repo.resolve_kb(kb_param)
    if not kb then return nil, { code = M.ERR.BAD_REQUEST, message = "Knowledge base not found: " .. kb_param } end
    return kb, nil
end

-- KB operations

function M.create_kb(params)
    if not params or not params.name or params.name == "" then
        return fail(M.ERR.BAD_REQUEST, "Name is required")
    end
    local kb, err = kb_repo.create_kb({ name = params.name, description = params.description })
    if err then return fail(M.ERR.INTERNAL, err) end
    return kb
end

function M.list_kbs()
    local kbs, err = kb_repo.list_kbs()
    if err then return fail(M.ERR.INTERNAL, err) end
    return kbs or {}
end

function M.delete_kb(id)
    if not id or id == "" then return fail(M.ERR.BAD_REQUEST, "KB ID required") end
    local result, err = kb_repo.delete_kb(id)
    if err then return fail(M.ERR.INTERNAL, err) end
    return result
end

-- Node operations

function M.create_node(params)
    if not params or not params.title or params.title == "" then
        return fail(M.ERR.BAD_REQUEST, "Title is required")
    end

    local kb, kb_err = resolve_kb_id(params.kb)
    if kb_err then return nil, kb_err end

    local node, err = kb_repo.create({
        kb_id = kb and kb.id,
        node_type = params.node_type or consts.NODE_TYPE.PATTERN,
        title = params.title,
        content = params.content or "",
        source = params.source or consts.SOURCE.HUMAN,
        confidence = params.confidence or 1.0,
        parent_id = params.parent_id,
        refs = params.refs,
        metadata = params.metadata,
    })
    if err or not node then return fail(M.ERR.INTERNAL, err or "create failed") end

    pcall(function() embedder_mod.embed({ node_id = node.id }) end)
    return node
end

function M.update_node(id, params)
    if not id or id == "" then return fail(M.ERR.BAD_REQUEST, "Node ID required") end
    if not params then return fail(M.ERR.BAD_REQUEST, "Update body required") end
    local result, err = kb_repo.update(id, params)
    if err then return fail(M.ERR.INTERNAL, err) end
    return result
end

function M.delete_node(id)
    if not id or id == "" then return fail(M.ERR.BAD_REQUEST, "Node ID required") end
    local result, err = kb_repo.delete(id)
    if err then return fail(M.ERR.INTERNAL, err) end
    return result
end

function M.get_node(id)
    if not id or id == "" then return fail(M.ERR.BAD_REQUEST, "Node ID required") end
    local node = kb_repo.get(id)
    if not node then return fail(M.ERR.NOT_FOUND, "Node not found") end
    return node
end

function M.list_nodes(params)
    params = params or {}
    local kb, kb_err = resolve_kb_id(params.kb)
    if kb_err then return nil, kb_err end

    local nodes, err = kb_repo.list({
        kb_id = kb and kb.id,
        node_type = params.node_type,
        source = params.source,
        parent_id = params.parent_id,
        limit = params.limit or 200,
    })
    if err then return fail(M.ERR.INTERNAL, err) end
    return nodes or {}
end

-- Search

function M.search_text(params)
    if not params or not params.query or params.query == "" then
        return fail(M.ERR.BAD_REQUEST, "Query is required")
    end
    local kb, kb_err = resolve_kb_id(params.kb)
    if kb_err then return nil, kb_err end

    local nodes, err = kb_repo.search_text(params.query, {
        kb_id = kb and kb.id,
        limit = params.limit or 20,
    })
    if err then return fail(M.ERR.INTERNAL, err) end
    return nodes or {}
end

function M.search_semantic(params)
    if not params or not params.query or params.query == "" then
        return fail(M.ERR.BAD_REQUEST, "Query is required")
    end

    local embed_result = nil
    local embed_err = nil
    local model = nil
    for _, candidate in ipairs(consts.embedding_models(params.model)) do
        embed_result, embed_err = llm.embed(params.query, {
            model = candidate,
            dimensions = consts.EMBED.DIMENSIONS,
        })
        if embed_result then
            model = candidate
            break
        end
    end
    if embed_err or not embed_result then
        return fail(M.ERR.INTERNAL, "Embedding failed: " .. (embed_err or "no result"))
    end

    local result_vec = embed_result.result
    if not result_vec then return fail(M.ERR.INTERNAL, "Empty embedding result") end
    local query_embedding = type(result_vec) == "table" and result_vec[1] or result_vec

    local kb, kb_err = resolve_kb_id(params.kb)
    if kb_err then return nil, kb_err end

    local nodes, search_err = kb_repo.search_by_embedding(query_embedding, {
        kb_id = kb and kb.id,
        limit = params.limit or 10,
    })
    if search_err then return fail(M.ERR.INTERNAL, search_err) end
    return { nodes = nodes or {}, model = model }
end

-- Stats

function M.stats(params)
    params = params or {}
    local kb, kb_err = resolve_kb_id(params.kb)
    if kb_err then return nil, kb_err end
    local stats, err = kb_repo.stats({ kb_id = kb and kb.id })
    if err then return fail(M.ERR.INTERNAL, err) end
    return stats
end

-- Research orchestration

local function with_kb_instruction(prompt, kb_name)
    local instruction = "This is a durable knowledge curation flow, not task-context research.\n" ..
        "Search existing knowledge first, then create or update concise durable nodes with write_knowledge.\n" ..
        "Do not use save_context; task-scoped context is owned by Keeper task phases.\n"

    if kb_name and kb_name ~= "" then
        instruction = instruction ..
            "TARGET KB: \"" .. kb_name .. "\"\n" ..
            "All write_knowledge calls in this task MUST set kb=\"" .. kb_name .. "\".\n"
    end

    return instruction .. "\n" .. prompt
end

local function resolve_kb_name(kb_param)
    local kb, err = resolve_kb_id(kb_param)
    if err then return nil, err end
    return kb and kb.name, nil
end

local function knowledge_agent_id()
    return KNOWLEDGE_CURATOR_AGENT
end

function M.research(params)
    params = params or {}
    local kb_name, kb_err = resolve_kb_name(params.kb)
    if kb_err then return nil, kb_err end

    if params.prompt and params.prompt ~= "" then
        local agent_prompt = with_kb_instruction(params.prompt, kb_name)
        local f = flow.create()
            :with_title("Research: " .. params.prompt:sub(1, 60))
            :with_metadata({ type = "knowledge_research", source = "keeper", target_kb = kb_name })
            :with_input({ prompt = agent_prompt, kb = kb_name })

        f:agent(knowledge_agent_id(), {
            arena = {
                prompt = agent_prompt,
                max_iterations = params.max_iterations or 15,
            },
        })

        local dataflow_id, flow_err = f:start()
        if flow_err then return fail(M.ERR.INTERNAL, "Failed to start: " .. flow_err) end
        return { dataflow_id = dataflow_id, mode = "single", target_kb = kb_name }
    end

    if params.prompts and type(params.prompts) == "table" and #params.prompts > 0 then
        local template = flow.template()
        template:agent(knowledge_agent_id(), {
            arena = {
                prompt = "{{item.prompt}}",
                max_iterations = params.max_iterations or 15,
            },
        })

        local items = {}
        for _, p in ipairs(params.prompts) do
            if type(p) == "string" and p ~= "" then
                table.insert(items, { prompt = with_kb_instruction(p, kb_name) })
            end
        end
        if #items == 0 then return fail(M.ERR.BAD_REQUEST, "No valid prompts") end

        local f = flow.create()
            :with_title("Batch Research (" .. #items .. " topics)")
            :with_metadata({
                type = "knowledge_research_batch",
                source = "keeper",
                topic_count = #items,
                target_kb = kb_name,
            })
            :with_input({ topics = items, kb = kb_name })

        f:parallel({
            source_array_key = "topics",
            iteration_input_key = "item",
            batch_size = params.batch_size or 5,
            on_error = "continue",
            template = template,
            metadata = { title = "Research Topics" },
        })

        local dataflow_id, flow_err = f:start()
        if flow_err then return fail(M.ERR.INTERNAL, "Failed to start batch: " .. flow_err) end
        return { dataflow_id = dataflow_id, mode = "batch", topics = #items, target_kb = kb_name }
    end

    return fail(M.ERR.BAD_REQUEST, "Provide 'prompt' (single) or 'prompts' (array)")
end

-- Registry analysis for `learn`. Reads kind/meta.type/namespace distribution
-- from the state DB so prompts can target the shapes actually present in
-- this installation rather than a fixed taxonomy.

local KIND_PROMPTS = {
    ["function.lua"] = "Explore function.lua entry patterns: required YAML fields (modules, imports, method, pool), Lua handler structure, return patterns. Read 3-4 diverse examples with get_entries. Check Wippy docs for available modules.",
    ["library.lua"] = "Explore library.lua patterns: module table (local M = {} / return M), how to get DB access via sql.get(), imports vs modules. Read 3-4 examples.",
    ["http.endpoint"] = "Explore http.endpoint patterns: how endpoints pair with function handlers, method/func/path fields, router reference. Read actual endpoint+handler pairs.",
    ["registry.entry"] = "Explore registry.entry patterns by meta.type: agents (agent.gen1), traits (agent.trait), models (llm.model), views (view.page). Read examples of each meta.type.",
    ["process.lua"] = "Explore process.lua patterns: auto_start, lifecycle, process.receive loop, process.send messaging. Read actual process entries.",
    ["contract.definition"] = "Explore contract patterns: definition with method schemas, binding that maps methods to functions. Read definition+binding pairs.",
    ["http.router"] = "Explore http.router patterns: prefix, middleware chain, CORS options, how routers connect to endpoints. Read router entries.",
    ["env.variable"] = "Explore env.variable patterns: storage backends (file, os, memory), how env variables are declared and accessed.",
    ["db.sql.sqlite"] = "Explore database patterns: db.sql.sqlite declaration, how migration entries reference target_db, how code accesses DB via sql.get().",
    ["security.policy"] = "Explore security.policy patterns: group definitions, rules, how policies are applied. Read actual policy entries.",
}

local META_PROMPTS = {
    ["agent.gen1"] = "Explore agent.gen1 in depth: prompt structure, model selection, traits list, tools list, delegates, class tags, temperature/max_tokens. Read all agent entries.",
    ["tool"] = "Explore tool meta.type: input_schema JSON format, llm_alias, llm_description, handler function pattern. Read tool entries and check docs.",
    ["agent.trait"] = "Explore agent.trait meta.type: how traits bundle tools, prompt injection via trait prompts. Read trait entries.",
    ["migration"] = "Explore migration meta.type: target_db field, up/down functions, table creation patterns, index patterns. Read migration entries.",
    ["view.page"] = "Explore view.page patterns: url field, proxy configuration, how views are served. Read view page entries.",
}

local function analyze_registry()
    local db = get_state_db()
    if not db then return nil, "State DB not available" end

    local kinds = db:query([[
        SELECT kind, COUNT(*) as cnt FROM keeper_overlay_entries
        WHERE branch = 'main' AND deleted = 0
        GROUP BY kind ORDER BY cnt DESC
    ]])

    local meta_types = db:query([[
        SELECT attr_value as meta_type, COUNT(*) as cnt FROM keeper_overlay_attributes
        WHERE branch = 'main' AND attr_key = 'meta.type'
        GROUP BY attr_value ORDER BY cnt DESC
    ]])

    local namespaces = db:query([[
        SELECT CASE
            WHEN INSTR(id, '.') > 0 THEN SUBSTR(id, 1, INSTR(id, '.') - 1)
            ELSE SUBSTR(id, 1, INSTR(id, ':') - 1)
        END as root_ns,
        COUNT(*) as cnt
        FROM keeper_overlay_entries WHERE branch = 'main' AND deleted = 0
        GROUP BY root_ns ORDER BY cnt DESC
    ]])

    return {
        kinds = kinds or {},
        meta_types = meta_types or {},
        namespaces = namespaces or {},
    }
end

local function build_learn_prompts(analysis)
    local prompts = {}
    for _, row in ipairs(analysis.kinds) do
        local p = KIND_PROMPTS[row.kind]
        if p and row.cnt >= 2 then table.insert(prompts, p) end
    end
    for _, row in ipairs(analysis.meta_types) do
        local p = META_PROMPTS[row.meta_type]
        if p and row.cnt >= 1 then table.insert(prompts, p) end
    end
    for _, row in ipairs(analysis.namespaces) do
        if row.cnt >= 10 and row.root_ns ~= "wippy" and row.root_ns ~= "userspace" then
            table.insert(prompts, "Explore the " .. row.root_ns ..
                " namespace hierarchy: what sub-namespaces exist, how they organize, naming conventions. Use explore_state tree operation.")
        end
    end

    local seen, unique = {}, {}
    for _, p in ipairs(prompts) do
        if not seen[p] then
            seen[p] = true
            table.insert(unique, p)
        end
    end
    return unique
end

function M.learn(params)
    params = params or {}
    local kb_name, kb_err = resolve_kb_name(params.kb)
    if kb_err then return nil, kb_err end

    local analysis, analysis_err = analyze_registry()
    if not analysis then return fail(M.ERR.INTERNAL, analysis_err) end

    local prompts = build_learn_prompts(analysis)
    local max_topics = tonumber(params.max_topics) or 25
    if #prompts > max_topics then
        local capped = {}
        for i = 1, max_topics do capped[i] = prompts[i] end
        prompts = capped
    end

    if #prompts == 0 then
        return {
            dataflow_id = nil,
            topics = 0,
            prompts = {},
            target_kb = kb_name,
            message = "No research topics generated",
        }
    end

    local template = flow.template()
    template:agent(knowledge_agent_id(), {
        arena = { prompt = "{{item.prompt}}", max_iterations = 15 },
    })

    local items = {}
    for _, p in ipairs(prompts) do
        table.insert(items, { prompt = with_kb_instruction(p, kb_name) })
    end

    local f = flow.create()
        :with_title("Learn Project (" .. #items .. " topics)")
        :with_metadata({
            type = "knowledge_learn",
            source = "keeper",
            topic_count = #items,
            target_kb = kb_name,
        })
        :with_input({ topics = items, kb = kb_name })

    f:parallel({
        source_array_key = "topics",
        iteration_input_key = "item",
        batch_size = 3,
        on_error = "continue",
        template = template,
        metadata = { title = "Learn Project" },
    })

    local dataflow_id, flow_err = f:start()
    if flow_err then return fail(M.ERR.INTERNAL, "Failed to start: " .. flow_err) end

    return {
        dataflow_id = dataflow_id,
        topics = #prompts,
        prompts = prompts,
        target_kb = kb_name,
    }
end

M._test = {
    knowledge_agent_id = knowledge_agent_id,
    with_kb_instruction = with_kb_instruction,
}

return M

