local uuid = require("uuid")
local json = require("json")
local process = require("process")
local execution_identity = require("execution_identity")
local consts = require("quantum_consts")
local idea_thread = require("quantum_idea_thread")
local repo = require("quantum_repo")
local content = require("quantum_content")
local writer = require("quantum_writer")
local config = require("quantum_config")
local agent_run = require("quantum_agent_run")
local schemas = require("quantum_schemas")
local patch = require("quantum_patch")
local kb_read = require("quantum_kb_read")

-- The idea aggregate command seam. Runs under the caller's actor. Each command
-- appends one event onto the idea thread; the declared projection folds it into
-- the read model. create/source/angle_set/angle_promote are the no-LLM commands;
-- generate_angles/draft/draft_variants/run_critics/apply_edit/mark_review_ready/
-- approve/reject are the review pipeline — each runs its role agent through the
-- injected `run` seam (agent_run.run by default) so a test never needs a live
-- model.
local M = {}

-- One critic call per concern, so one critic agent serves multiple judgments.
-- category falls back to the concern key when the model omits it.
local CRITIC_CONCERNS = {
    { key = "voice", label = "Voice",
      instruction = "Flag any sentence that reads like a detached commentator or an AI explainer rather than a first-person practitioner speaking from experience: abstract openers, textbook framing, \"we\" where a real author would say \"I\", explaining the obvious." },
    { key = "anti_slop", label = "Anti-slop / AI-tells",
      instruction = "Flag AI-tell phrasing: hedges (\"can\", \"often\", \"may\"), empty transitions (\"what actually happens is\", \"the reality is\", \"at the end of the day\"), cliches and filler (\"game-changer\", \"unpopular opinion\", \"in today's world\", \"the key is\"), listy parallelism, and em dashes used as connective tissue. Name the exact phrase." },
    { key = "claim_support", label = "Claim support",
      instruction = "Flag any claim in the prose that is not supported by the source material or the document's own claims/evidence — overstated numbers, asserted causation, or generalizations the source does not back." },
    { key = "pulse", label = "Human pulse",
      instruction = "This concern is the OPPOSITE of slop-hunting: flag prose that is flat, emotionless, and reads like a neutral DIGEST of facts rather than a person who lived it. Flag: a scene told from the outside instead of put us inside it; no stakes named (why it matters to a real person); no conviction or edge (a neutral reporter, not someone with an opinion he'd bet on); monotone rhythm (every sentence the same length and shape); telling us the point instead of making us feel it. A technically-correct, slop-free paragraph with no pulse is a defect here. Quote the flattest line." },
}

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function enc(value: any): string
    local out, err = json.encode(value)
    if err or type(out) ~= "string" then return "[]" end
    return out
end

-- establish(idea_id, injected?) -> (est, err). Resolve the caller + the idea
-- id + its thread. injected is { exec, actor_id }, supplied by a caller that
-- already holds a reconstructed identity (draft_variants_process, restoring
-- the request-time actor frozen via execution_identity) instead of the
-- ambient ns actor idea_thread.executor() reads. Every other caller runs
-- under the request's own actor, so injected is nil and behavior is
-- unchanged.
local function establish(idea_id: string, injected: any?): (any?, string?)
    local exec, actor_id
    if type(injected) == "table" and injected.exec and type(injected.actor_id) == "string" and injected.actor_id ~= "" then
        exec, actor_id = injected.exec, injected.actor_id
    else
        local eerr
        exec, actor_id, eerr = idea_thread.executor()
        if eerr or not exec or not actor_id then return nil, eerr end
    end
    local id = trim(idea_id) ~= "" and trim(idea_id) or uuid.v7()
    return { exec = exec, actor_id = actor_id, idea_id = id }, nil
end

-- resolve(idea_id, injected?) -> (est, thread_id, idea, err). Shared preamble
-- for every command that acts on an already-created idea: establish the
-- caller, resolve the thread, and load the current read-model row.
local function resolve(idea_id: string, injected: any?): (any?, string?, any?, string?)
    local est, err = establish(idea_id, injected)
    if not est then return nil, nil, nil, err end
    local e: any = est
    local thread_id, terr = idea_thread.resolve(e.exec, e.actor_id, idea_id)
    if not thread_id then return nil, nil, nil, terr end
    local idea, gerr = repo.get_idea(idea_id)
    if gerr then return nil, nil, nil, gerr end
    if not idea then return nil, nil, nil, "idea not found" end
    return e, thread_id, idea, nil
end

-- prose_blocks(doc) -> [{id, text}]. The only shape critics/editor ever see.
local function prose_blocks(doc: any): { any }
    local blocks: { any } = {}
    local prose: any = type(doc) == "table" and doc.prose or {}
    for _, p in ipairs(type(prose) == "table" and prose or {}) do
        blocks[#blocks + 1] = { id = trim((p :: any).id), text = trim((p :: any).text) }
    end
    return blocks
end

-- create(input) -> (result, err). input: { title?, content_type?, advance_mode?,
-- profile_id?, profile_ver?, idea_id? }. Establishes the idea thread and appends
-- the genesis CREATED event.
function M.create(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local est, err = establish(trim((input :: any).idea_id))
    if not est then return { ok = false }, err end
    local e: any = est

    local thread_id, terr = idea_thread.ensure(e.exec, e.actor_id, e.idea_id, trim((input :: any).title))
    if not thread_id then return { ok = false }, terr end

    local advance_mode = trim((input :: any).advance_mode)
    if advance_mode ~= consts.ADVANCE_MODE.AUTOPILOT then advance_mode = consts.ADVANCE_MODE.MANUAL end

    local aerr = idea_thread.append(e.exec, thread_id, consts.EVENT.CREATED, {
        idea_id = e.idea_id,
        actor_id = e.actor_id,
        title = trim((input :: any).title),
        content_type = trim((input :: any).content_type),
        advance_mode = advance_mode,
        profile_id = trim((input :: any).profile_id),
        profile_ver = trim((input :: any).profile_ver),
    }, "user")
    if aerr then return { ok = false }, "could not append idea_created: " .. aerr end

    return { ok = true, idea_id = e.idea_id, thread_id = thread_id, status = consts.STATUS.DRAFTING }, nil
end

-- source_paste(input) -> (result, err). input: { idea_id, text, provenance? }.
function M.source_paste(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local est, err = establish(idea_id)
    if not est then return { ok = false }, err end
    local e: any = est
    local thread_id, terr = idea_thread.resolve(e.exec, e.actor_id, idea_id)
    if not thread_id then return { ok = false }, terr end

    local aerr = idea_thread.append(e.exec, thread_id, consts.EVENT.SOURCE_PASTED, {
        idea_id = idea_id,
        source_id = uuid.v7(),
        text = trim((input :: any).text),
        provenance = (input :: any).provenance,
    }, "user")
    if aerr then return { ok = false }, "could not append source_pasted: " .. aerr end
    return { ok = true, idea_id = idea_id }, nil
end

-- import(input) -> (result, err). input: { external_key, title?, content_type?,
-- source_text? }. Idempotent create-or-return by a caller-supplied natural
-- key, for a sibling module that never wants to duplicate an idea it already
-- imported. A match in the read model returns it unchanged (created=false);
-- otherwise creates the idea and folds the source in the same call (CREATED
-- carrying external_key, then SOURCE_PASTED), returning created=true. A
-- racing import for the same key is resolved by the UNIQUE index on
-- external_key at the projection fold (repo.apply): the losing insert is
-- absorbed there since the winner already holds the canonical row.
function M.import(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local external_key = trim((input :: any).external_key)
    if external_key == "" then return { ok = false }, "external_key is required" end

    local est, err = establish("")
    if not est then return { ok = false }, err end
    local e: any = est

    local existing, gerr = repo.get_by_external_key(e.actor_id, external_key)
    if gerr then return { ok = false }, gerr end
    if existing then return { ok = true, idea_id = (existing :: any).idea_id, created = false }, nil end

    local title = trim((input :: any).title)
    local thread_id, terr = idea_thread.ensure(e.exec, e.actor_id, e.idea_id, title)
    if not thread_id then return { ok = false }, terr end

    local aerr = idea_thread.append(e.exec, thread_id, consts.EVENT.CREATED, {
        idea_id = e.idea_id,
        actor_id = e.actor_id,
        title = title,
        content_type = trim((input :: any).content_type),
        advance_mode = consts.ADVANCE_MODE.MANUAL,
        external_key = external_key,
    }, "user")
    if aerr then return { ok = false }, "could not append idea_created: " .. aerr end

    local serr = idea_thread.append(e.exec, thread_id, consts.EVENT.SOURCE_PASTED, {
        idea_id = e.idea_id,
        source_id = uuid.v7(),
        text = trim((input :: any).source_text),
    }, "user")
    if serr then return { ok = false }, "could not append source_pasted: " .. serr end

    return { ok = true, idea_id = e.idea_id, created = true }, nil
end

-- angle_set(input) -> (result, err). input: { idea_id, angles }. Records the
-- candidate angles for the author (or autopilot) to promote one.
function M.angle_set(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local est, err = establish(idea_id)
    if not est then return { ok = false }, err end
    local e: any = est
    local thread_id, terr = idea_thread.resolve(e.exec, e.actor_id, idea_id)
    if not thread_id then return { ok = false }, terr end

    local aerr = idea_thread.append(e.exec, thread_id, consts.EVENT.ANGLE_SET, {
        idea_id = idea_id,
        angles = type((input :: any).angles) == "table" and (input :: any).angles or {},
    })
    if aerr then return { ok = false }, "could not append angle_set: " .. aerr end
    return { ok = true, idea_id = idea_id }, nil
end

-- angle_promote(input) -> (result, err). input: { idea_id, angle_id, take }. The
-- one HARD creative-commitment gate before drafting.
function M.angle_promote(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    local take = trim((input :: any).take)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    if take == "" then return { ok = false }, "a promoted angle needs a take" end
    local est, err = establish(idea_id)
    if not est then return { ok = false }, err end
    local e: any = est
    local thread_id, terr = idea_thread.resolve(e.exec, e.actor_id, idea_id)
    if not thread_id then return { ok = false }, terr end

    local aerr = idea_thread.append(e.exec, thread_id, consts.EVENT.ANGLE_PROMOTED, {
        idea_id = idea_id,
        angle_id = trim((input :: any).angle_id),
        take = take,
    }, "user")
    if aerr then return { ok = false }, "could not append angle_promoted: " .. aerr end
    return { ok = true, idea_id = idea_id, status = consts.STATUS.WRITING }, nil
end

-- generate_angles(input) -> (result, err). input: { idea_id, run?, read_kb?
-- }. Requires a pasted source. Runs the angles role once (a single, ~30-60s
-- model call — a foreground command, like draft) with the structured ANGLES
-- exit schema, in-voice via the same Content Bible + Anti-Slop Bible KB reads
-- draft uses, and appends the candidates as ONE ANGLE_SET event; the existing
-- fold already stores them onto angles_json. run/read_kb are injectable so a
-- test drives the whole command with stubs.
function M.generate_angles(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea
    local source = trim(id.source_text)
    if source == "" then return { ok = false }, "a source is required before generating angles" end

    local read_kb: any = type((input :: any).read_kb) == "function" and (input :: any).read_kb or kb_read.passages
    local voice = read_kb(config.voice_kbs(), source, 6)
    local avoid = read_kb(config.anti_slop_kbs(), "AI tells and slop to avoid", 8)

    local message_parts: { string } = {}
    if voice ~= "" then message_parts[#message_parts + 1] = "VOICE (write in this voice):\n" .. voice end
    if avoid ~= "" then message_parts[#message_parts + 1] = "AVOID (never produce these AI-tells):\n" .. avoid end
    message_parts[#message_parts + 1] = "SOURCE MATERIAL:\n" .. source
    message_parts[#message_parts + 1] =
        "Propose 3 to 5 distinct candidate angles for this source, each a one-sentence take plus a short rationale."

    local run: any = type((input :: any).run) == "function" and (input :: any).run or agent_run.run
    local raw, rerr = run({
        agent_ref = config.angles_agent(),
        model = config.angles_model(),
        input = table.concat(message_parts, "\n\n"),
        exit_schema = schemas.ANGLES,
    })
    if rerr then
        idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED, { idea_id = idea_id, stage = "angles", error = tostring(rerr) })
        return { ok = false }, rerr
    end

    local raw_angles: any = type(raw) == "table" and raw.angles or {}
    local angles: { any } = {}
    for i, a in ipairs(type(raw_angles) == "table" and raw_angles or {}) do
        local ang: any = type(a) == "table" and a or {}
        local take = trim(ang.take)
        if take ~= "" then
            angles[#angles + 1] = {
                id = trim(ang.id) ~= "" and trim(ang.id) or ("a" .. tostring(i)),
                take = take,
                rationale = trim(ang.rationale),
            }
        end
    end
    if #angles == 0 then
        idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED,
            { idea_id = idea_id, stage = "angles", error = "the angles agent returned no usable angles" })
        return { ok = false }, "the angles agent returned no usable angles"
    end

    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.ANGLE_SET, {
        idea_id = idea_id,
        angles = angles,
    })
    if aerr then return { ok = false }, "could not append angle_set: " .. aerr end
    return { ok = true, idea_id = idea_id, count = #angles }, nil
end

-- draft(input) -> (result, err). input: { idea_id, run?, read_kb? }. Runs the
-- writer on the idea's promoted take + source and records the structured
-- draft. The writer's agent/model are deployment role settings; the voice
-- (Content Bible) and avoid (Anti-Slop Bible) passages come from the
-- configured KBs, and the target length/structure from the resolved
-- template. run and read_kb are injectable so a test drives the whole command
-- with stubs instead of a live model or a live KB.
function M.draft(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea
    if trim(id.take) == "" then return { ok = false }, "promote an angle before drafting" end

    local read_kb: any = type((input :: any).read_kb) == "function" and (input :: any).read_kb or kb_read.passages
    local voice = read_kb(config.voice_kbs(), id.take, 6)
    local avoid = read_kb(config.anti_slop_kbs(), "AI tells and slop to avoid", 8)
    local template = config.template_by_id(id.content_type)

    local run: any = type((input :: any).run) == "function" and (input :: any).run or agent_run.run
    local doc, werr = writer.draft({
        take = id.take,
        source = id.source_text,
        content_type = id.content_type,
        voice = voice,
        avoid = avoid,
        template_spec = type(template) == "table" and trim((template :: any).spec) or "",
        house_rules = config.house_rules(),
        model = config.writer_model(),
        agent_ref = config.writer_agent(),
        exit_schema = schemas.DOCUMENT,
        run = run,
    })
    if not doc then
        idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED, { idea_id = idea_id, stage = "writer", error = tostring(werr) })
        return { ok = false }, werr
    end

    local revision_id = writer.revision_id()
    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.DRAFT_RECORDED, {
        idea_id = idea_id,
        revision_id = revision_id,
        document = doc,
        doc_hash = content.doc_hash(doc),
        take = doc.take,
    })
    if aerr then return { ok = false }, "could not append draft_recorded: " .. aerr end
    return { ok = true, idea_id = idea_id, revision_id = revision_id, status = consts.STATUS.REVIEWING }, nil
end

-- Fixed set of opening angles a draft_variants batch cycles through, so N
-- writer runs diverge on one axis (how the piece opens) and nothing else.
local VARIANT_ANGLES = {
    "Open on a concrete scene or moment.",
    "Open with the blunt claim, no runway.",
    "Open on a specific number, name, or artifact.",
}

-- resolve_selected_angles(angles, angle_ids) -> [{id,take,rationale}], in the
-- order angle_ids were given. angles is the idea's read-model `angles` field
-- (the ANGLE_SET candidates); an id with no match is silently dropped so a
-- stale/typo'd id never blows up the whole batch.
local function resolve_selected_angles(angles: any, angle_ids: any): { any }
    local by_id: { [string]: any } = {}
    for _, a in ipairs(type(angles) == "table" and angles or {}) do
        local aid = trim((a :: any).id)
        if aid ~= "" then by_id[aid] = a end
    end
    local out: { any } = {}
    for _, raw_id in ipairs(type(angle_ids) == "table" and angle_ids or {}) do
        local found = by_id[trim(tostring(raw_id))]
        if found then out[#out + 1] = found end
    end
    return out
end

-- draft_variants(input) -> (result, err). input: { idea_id, n?, angle_ids?,
-- run?, read_kb?, exec?, actor_id? }. Two modes: with angle_ids (a non-empty
-- array), runs the writer once per SELECTED angle, each pinned to that
-- angle's own take (not the idea's promoted take — a selected angle need
-- never have been promoted) with the angle's rationale as the variant label.
-- Without angle_ids, runs the writer n times (default 3, clamped 2..4)
-- against the idea's promoted take, each pinned to a distinct fixed opening
-- angle — the original diversity-hint behavior, unchanged. A run that fails
-- is skipped, not fatal; only if every run fails does the command record
-- JOB_FAILED and return the error. Every successful run is folded into ONE
-- VARIANTS_RECORDED event, leaving status/stage untouched — a variant is a
-- candidate, not the chosen draft. exec/actor_id are the injected-identity
-- seam draft_variants_process supplies after reconstructing the caller
-- frozen at request time; every other caller (tests, the exec:call wrapper)
-- omits them and runs under its own ambient actor.
function M.draft_variants(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local injected: any = nil
    if (input :: any).exec ~= nil and type((input :: any).actor_id) == "string" and (input :: any).actor_id ~= "" then
        injected = { exec = (input :: any).exec, actor_id = (input :: any).actor_id }
    end
    local e, thread_id, idea, err = resolve(idea_id, injected)
    if not e then return { ok = false }, err end
    local id: any = idea

    local angle_ids: any = (input :: any).angle_ids
    local selected: { any }? = nil
    if type(angle_ids) == "table" and #angle_ids > 0 then
        selected = resolve_selected_angles(id.angles, angle_ids)
        if #(selected :: { any }) == 0 then return { ok = false }, "no matching angles found for the given angle_ids" end
    elseif trim(id.take) == "" then
        return { ok = false }, "promote an angle before drafting"
    end

    local read_kb: any = type((input :: any).read_kb) == "function" and (input :: any).read_kb or kb_read.passages
    local avoid = read_kb(config.anti_slop_kbs(), "AI tells and slop to avoid", 8)
    local template = config.template_by_id(id.content_type)

    local run: any = type((input :: any).run) == "function" and (input :: any).run or agent_run.run

    -- runs: [{ take, label, variant_angle? }]. angle-driven: one run per
    -- selected angle, take = the angle's own take, label = its rationale.
    -- fixed-hint: n runs, all sharing the idea's promoted take, each pinned to
    -- a distinct opening angle via variant_angle (label doubles as that hint).
    local runs: { any } = {}
    if selected then
        for _, a in ipairs(selected :: { any }) do
            local take = trim((a :: any).take)
            local rationale = trim((a :: any).rationale)
            runs[#runs + 1] = { take = take, label = rationale ~= "" and rationale or take }
        end
    else
        local n = math.floor(tonumber((input :: any).n) or 3)
        n = math.max(2, math.min(4, n))
        for i = 1, n do
            local angle = VARIANT_ANGLES[((i - 1) % #VARIANT_ANGLES) + 1]
            runs[#runs + 1] = { take = id.take, label = angle, variant_angle = angle }
        end
    end

    local variants: { any } = {}
    local last_err: string? = nil
    for _, r in ipairs(runs) do
        local take = trim((r :: any).take)
        -- Voice passages are keyed on THIS run's own take, so an angle-driven
        -- variant's prose is judged in-voice against the angle it is actually
        -- drafting, not the idea's (possibly unset) promoted take.
        local voice = read_kb(config.voice_kbs(), take, 6)
        local doc, werr = writer.draft({
            take = take,
            source = id.source_text,
            content_type = id.content_type,
            voice = voice,
            avoid = avoid,
            template_spec = type(template) == "table" and trim((template :: any).spec) or "",
            house_rules = config.house_rules(),
            variant_angle = (r :: any).variant_angle,
            model = config.writer_model(),
            agent_ref = config.writer_agent(),
            exit_schema = schemas.DOCUMENT,
            run = run,
        })
        if doc then
            variants[#variants + 1] = {
                variant_id = uuid.v7(),
                label = (r :: any).label,
                document = doc,
                doc_hash = content.doc_hash(doc),
            }
        else
            last_err = tostring(werr)
        end
    end

    if #variants == 0 then
        idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED,
            { idea_id = idea_id, stage = "writer_variants", error = tostring(last_err) })
        return { ok = false }, last_err
    end

    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.VARIANTS_RECORDED, {
        idea_id = idea_id,
        variants = variants,
    })
    if aerr then return { ok = false }, "could not append variants_recorded: " .. aerr end
    return { ok = true, idea_id = idea_id, count = #variants }, nil
end

-- The production start_job seam: spawns draft_variants_process as a
-- TOP-LEVEL background process (process.spawn, not a dataflow). The writer
-- role drives its structured-output run through its own single-step flow
-- (agent_run.run -> flow.create():agent():run()); a flow run started from
-- inside another flow's func node is nested and returns nothing ("the
-- writer returned no prose"), so draft_variants can never run as a
-- dataflow node itself. A detached spawned process carries no ambient
-- security actor, so the caller's identity is frozen here (still inside the
-- request's own actor/scope) via execution_identity.capture and handed into
-- the process as spawn args; draft_variants_process reconstructs it before
-- calling aggregate.draft_variants, so repo reads and event appends run as
-- the user who asked for the batch, not as no one.
local function start_variants_job(idea_id: string, n: number, angle_ids: any): (string?, string?)
    local identity, cerr = execution_identity.capture("spiralscout.quantum")
    if cerr or not identity then return nil, "could not capture execution identity: " .. tostring(cerr) end
    local row, rerr = execution_identity.to_row(identity)
    if rerr or not row then return nil, "could not serialize execution identity: " .. tostring(rerr) end

    local host, herr = consts.process_host()
    if herr or not host then return nil, herr end

    local pid, spawn_err = process.spawn("spiralscout.quantum:draft_variants_process", host, {
        idea_id = idea_id,
        n = n,
        angle_ids = type(angle_ids) == "table" and angle_ids or nil,
        actor_id = row.actor_id,
        actor_context = row.actor_context,
    })
    if not pid then return nil, tostring(spawn_err or "process spawn failed") end
    return tostring(pid), nil
end

-- variants_request(input) -> (result, err). input: { idea_id, n?, angle_ids?,
-- start_job? }. The HTTP entry point for a variants batch. draft_variants
-- runs the writer against a real model (60-90s per run); running that chain
-- inline inside the request handler blocks the HTTP request for minutes and
-- the caller's gateway/proxy times it out and disconnects long before the
-- pipeline appends VARIANTS_RECORDED, so nothing lands. variants_request
-- does the fast part synchronously (validates the idea is ready — either a
-- promoted take, or angle_ids that actually resolve against the idea's
-- angles — and appends a VARIANTS_REQUESTED marker the projection folds into
-- variants_generating=true) and starts draft_variants as a background
-- process through the injected start_job seam (start_variants_job by
-- default; a test passes a stub so the kickoff is provable without a live
-- process/model). The request returns as soon as the job is spawned; the job appends
-- VARIANTS_RECORDED (or JOB_FAILED) when it finishes, same as if
-- draft_variants had run inline.
function M.variants_request(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea

    local angle_ids: any = (input :: any).angle_ids
    local has_angle_ids = type(angle_ids) == "table" and #angle_ids > 0
    if has_angle_ids then
        if #resolve_selected_angles(id.angles, angle_ids) == 0 then
            return { ok = false }, "no matching angles found for the given angle_ids"
        end
    elseif trim(id.take) == "" then
        return { ok = false }, "promote an angle before drafting"
    end

    local n = math.floor(tonumber((input :: any).n) or 3)
    n = math.max(2, math.min(4, n))

    local rerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.VARIANTS_REQUESTED,
        { idea_id = idea_id, n = has_angle_ids and #angle_ids or n })
    if rerr then return { ok = false }, "could not append variants_requested: " .. rerr end

    local start_job: any = type((input :: any).start_job) == "function" and (input :: any).start_job or start_variants_job
    local dataflow_id, ferr = start_job(idea_id, n, angle_ids)

    if ferr or not dataflow_id then
        idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED,
            { idea_id = idea_id, stage = "writer_variants", error = tostring(ferr or "could not start the variants job") })
        return { ok = false }, "variants start failed: " .. tostring(ferr)
    end

    return { ok = true, idea_id = idea_id, status = trim(id.status), generating = true }, nil
end

-- pick_variant(input) -> (result, err). input: { idea_id, variant_id }. Finds
-- the matching recorded variant and appends the SAME DRAFT_RECORDED event the
-- single draft command uses, so the fold moves status to reviewing with the
-- chosen document — the rest of the review pipeline never needs to know the
-- draft came from a batch.
function M.pick_variant(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    local variant_id = trim((input :: any).variant_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    if variant_id == "" then return { ok = false }, "variant_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea

    local variants: any = type(id.variants) == "table" and id.variants or {}
    local found: any = nil
    for _, v in ipairs(variants) do
        if trim((v :: any).variant_id) == variant_id then found = v; break end
    end
    if not found then return { ok = false }, "variant not found" end

    local revision_id = writer.revision_id()
    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.DRAFT_RECORDED, {
        idea_id = idea_id,
        revision_id = revision_id,
        document = found.document,
        doc_hash = found.doc_hash,
        -- The chosen variant's own take: an angle-driven variant's take may
        -- never have been promoted onto the idea, so the fold must take it
        -- from here for the read model's `take` to match the chosen document.
        take = trim((found.document :: any).take),
    })
    if aerr then return { ok = false }, "could not append draft_recorded: " .. aerr end
    return { ok = true, idea_id = idea_id, revision_id = revision_id, status = consts.STATUS.REVIEWING }, nil
end

-- run_critics(input) -> (result, err). input: { idea_id, run?, read_kb? }.
-- Requires a recorded document. Runs the critic agent once per concern
-- (voice, anti_slop, claim_support), merges all findings, and appends ONE
-- FINDINGS_RECORDED. A failed concern aborts the whole run rather than
-- recording a partial finding set. The anti_slop concern judges against the
-- Anti-Slop Bible KB, the voice concern against the Content Bible KB;
-- claim_support carries no KB rubric. read_kb is injectable so a test never
-- hits a live KB.
function M.run_critics(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea
    if type(id.document) ~= "table" or trim(id.take) == "" then
        return { ok = false }, "a document is required before critics can run"
    end

    local run: any = type((input :: any).run) == "function" and (input :: any).run or agent_run.run
    local read_kb: any = type((input :: any).read_kb) == "function" and (input :: any).read_kb or kb_read.passages
    local blocks_json = enc(prose_blocks(id.document))

    local all_findings: { any } = {}
    for _, concern in ipairs(CRITIC_CONCERNS) do
        local message_parts: { string } = {
            ("TAKE:\n%s\n\nCONCERN: %s\n%s\n\nPROSE BLOCKS (json, id+text):\n%s")
                :format(id.take, concern.label, concern.instruction, blocks_json)
        }
        if concern.key == "anti_slop" then
            local rubric = read_kb(config.anti_slop_kbs(), "AI tells and slop patterns", 8)
            if rubric ~= "" then
                message_parts[#message_parts + 1] = "RUBRIC (flag any prose matching these):\n" .. rubric
            end
        elseif concern.key == "voice" then
            local voice_rubric = read_kb(config.voice_kbs(), id.take, 6)
            if voice_rubric ~= "" then
                message_parts[#message_parts + 1] = "VOICE RUBRIC:\n" .. voice_rubric
            end
        end
        local message = table.concat(message_parts, "\n\n")
        local raw, rerr = run({
            agent_ref = config.critic_agent(),
            model = config.critic_model(),
            input = message,
            exit_schema = schemas.FINDINGS,
        })
        if rerr then
            idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED,
                { idea_id = idea_id, stage = "critic:" .. concern.key, error = tostring(rerr) })
            return { ok = false }, rerr
        end

        local findings: any = type(raw) == "table" and raw.findings or {}
        for _, f in ipairs(type(findings) == "table" and findings or {}) do
            local finding: any = type(f) == "table" and f or {}
            all_findings[#all_findings + 1] = {
                block_id = trim(finding.block_id),
                severity = trim(finding.severity) ~= "" and trim(finding.severity) or "low",
                category = trim(finding.category) ~= "" and trim(finding.category) or concern.key,
                message = trim(finding.message),
                quote = trim(finding.quote),
            }
        end
    end

    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.FINDINGS_RECORDED, {
        idea_id = idea_id,
        doc_hash = trim(id.doc_hash),
        findings = all_findings,
    })
    if aerr then return { ok = false }, "could not append findings_recorded: " .. aerr end
    return { ok = true, idea_id = idea_id, findings_count = #all_findings }, nil
end

-- apply_edit(input) -> (result, err). input: { idea_id, run? }. Requires a
-- document + existing findings. Runs the editor agent for patches, then the
-- take-preservation validator; a rejected patch set never forces through —
-- it records JOB_FAILED and returns the validator's error.
function M.apply_edit(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea
    if type(id.document) ~= "table" or trim(id.take) == "" then
        return { ok = false }, "a document is required before editing"
    end
    local findings: any = type(id.findings) == "table" and id.findings or {}
    if #findings == 0 then return { ok = false }, "no findings to edit against" end

    local run: any = type((input :: any).run) == "function" and (input :: any).run or agent_run.run
    local message = ("PROSE BLOCKS (json, id+text):\n%s\n\nFINDINGS (json):\n%s\n\nReturn patches that fix ONLY what the findings raise, changing prose text alone.")
        :format(enc(prose_blocks(id.document)), enc(findings))
    local raw, rerr = run({
        agent_ref = config.editor_agent(),
        model = config.editor_model(),
        input = message,
        exit_schema = schemas.PATCHES,
    })
    if rerr then
        idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED, { idea_id = idea_id, stage = "editor", error = tostring(rerr) })
        return { ok = false }, rerr
    end
    local patches: any = type(raw) == "table" and raw.patches or {}

    local result, perr = patch.validate_apply(id.document, type(patches) == "table" and patches or {})
    if not result then
        idea_thread.append((e :: any).exec, thread_id, consts.EVENT.JOB_FAILED, { idea_id = idea_id, stage = "editor", error = tostring(perr) })
        return { ok = false }, perr
    end
    local applied: any = result

    local revision_id = writer.revision_id()
    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.PATCH_APPLIED, {
        idea_id = idea_id,
        from_hash = trim(id.doc_hash),
        to_hash = applied.doc_hash,
        revision_id = revision_id,
        document = applied.document,
    })
    if aerr then return { ok = false }, "could not append patch_applied: " .. aerr end
    return { ok = true, idea_id = idea_id, revision_id = revision_id }, nil
end

-- mark_review_ready(input) -> (result, err). input: { idea_id }. Appends
-- REVIEW_READY once a document revision exists to send to the human.
function M.mark_review_ready(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea
    if trim(id.doc_hash) == "" then return { ok = false }, "a document is required before marking review ready" end

    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.REVIEW_READY, {
        idea_id = idea_id,
        revision_id = trim(id.revision_id),
        doc_hash = trim(id.doc_hash),
    })
    if aerr then return { ok = false }, "could not append review_ready: " .. aerr end
    return { ok = true, idea_id = idea_id, status = consts.STATUS.REVIEW_READY }, nil
end

-- approve(input) -> (result, err). input: { idea_id }. Human-only: the final
-- approval gate.
function M.approve(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea

    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.FINAL_APPROVED, {
        idea_id = idea_id,
        revision_id = trim(id.revision_id),
        by = (e :: any).actor_id,
    }, "user")
    if aerr then return { ok = false }, "could not append final_approved: " .. aerr end
    return { ok = true, idea_id = idea_id, status = consts.STATUS.APPROVED }, nil
end

-- reject(input) -> (result, err). input: { idea_id, reason? }. Human-only.
function M.reject(input: any): (any, string?)
    input = type(input) == "table" and input or {}
    local idea_id = trim((input :: any).idea_id)
    if idea_id == "" then return { ok = false }, "idea_id is required" end
    local e, thread_id, idea, err = resolve(idea_id)
    if not e then return { ok = false }, err end
    local id: any = idea

    local aerr = idea_thread.append((e :: any).exec, thread_id, consts.EVENT.FINAL_REJECTED, {
        idea_id = idea_id,
        revision_id = trim(id.revision_id),
        reason = trim((input :: any).reason),
    }, "user")
    if aerr then return { ok = false }, "could not append final_rejected: " .. aerr end
    return { ok = true, idea_id = idea_id, status = consts.STATUS.REJECTED }, nil
end

return M
