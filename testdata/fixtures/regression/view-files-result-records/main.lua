local security = require("security")
local contract = require("contract")
local base64 = require("base64")
local json = require("json")
local artifact_repo = require("artifact_repo")

local DEFAULT_TEXT_LIMIT = 16 * 1024
local MAX_TEXT_LIMIT = 64 * 1024
-- Total text per call is bounded by TOKENS, not bytes, so a large document never
-- overflows a small-context model (an 8k-window model 400s on ~24KB of text). The
-- byte windows above still apply per file; this token budget is the binding ceiling
-- across the whole call. The agent reads more via offset/next_offset.
local APPROX_BYTES_PER_TOKEN = 4
local MAX_TOTAL_TEXT_TOKENS = 6000
local function est_tokens(byte_len: integer): integer
    return math.floor((byte_len + APPROX_BYTES_PER_TOKEN - 1) / APPROX_BYTES_PER_TOKEN)
end
local DEFAULT_ARTIFACT_LIST_LIMIT = 20
local MAX_ARTIFACT_LIST_LIMIT = 50
local MAX_SELECTED_ARTIFACTS = 6
local DEFAULT_ARTIFACT_REF_LIMIT = 4
local MAX_INLINE_IMAGE_BYTES = 1024 * 1024
local MAX_INLINE_IMAGE_BYTES_TOTAL = 2 * 1024 * 1024

local function is_image_type(content_type)
    return type(content_type) == "string" and string.match(content_type, "^image/") ~= nil
end

local function clamp_int(value: any, default: integer, min: integer, max: integer): integer
    local n = tonumber(value) or default
    if n < min then n = min end
    if n > max then n = max end
    return math.floor(n)
end

local function image_artifact_summary(artifact: any): any
    local metadata = type(artifact.metadata) == "table" and artifact.metadata or {}
    return {
        upload_id = artifact.upload_id,
        artifact_id = artifact.artifact_id,
        kind = artifact.kind,
        role = artifact.role,
        filename = artifact.label,
        content_type = artifact.mime_type,
        size = artifact.size or metadata.size or metadata.original_size,
        width = metadata.width,
        height = metadata.height,
        ordinal = artifact.ordinal,
        occurrence_count = artifact.occurrence_count,
        duplicate_count = artifact.duplicate_count,
    }
end

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

local function list_image_artifacts(upload_id: string, args: any): (any?, integer?, string?)
    local limit = clamp_int(args.artifact_limit, DEFAULT_ARTIFACT_LIST_LIMIT, 1, MAX_ARTIFACT_LIST_LIMIT)
    local offset = clamp_int(args.artifact_offset, 0, 0, 1000000000)
    local total, count_err = artifact_repo.count_artifact_parts(upload_id, { images_only = true })
    if count_err then return nil, nil, count_err end
    local artifacts, list_err = artifact_repo.list_artifact_parts(upload_id, limit, offset, { images_only = true })
    if list_err then return nil, nil, list_err end

    local images = {}
    for _, artifact in ipairs(artifacts or {}) do
        local mime_type = tostring(artifact.mime_type or "")
        if artifact.kind == "image" or is_image_type(mime_type) then
            images[#images + 1] = image_artifact_summary(artifact)
        end
    end
    local total_count: integer = math.floor(tonumber(total) or 0)
    return images, total_count, nil
end

local function artifact_ref_limit(ref: any): integer
    if type(ref) ~= "table" then return 1 end
    return clamp_int(ref.limit or ref.max_items, DEFAULT_ARTIFACT_REF_LIMIT, 1, MAX_SELECTED_ARTIFACTS)
end

local function read_window(args: any, content: string, remaining_tokens: integer): any
    local total_size = #content
    local offset = clamp_int(args.offset, 0, 0, total_size)
    local requested_limit = clamp_int(args.limit, DEFAULT_TEXT_LIMIT, 1, MAX_TEXT_LIMIT)
    local limit = requested_limit
    -- Cap the byte window to what the remaining token budget allows.
    local token_byte_ceiling = remaining_tokens * APPROX_BYTES_PER_TOKEN
    if token_byte_ceiling < limit then limit = token_byte_ceiling end

    if limit <= 0 then
        return {
            content = nil,
            total_size = total_size,
            content_offset = offset,
            content_limit = requested_limit,
            content_length = 0,
            content_truncated = offset < total_size,
            next_offset = offset < total_size and offset or nil,
            content_omitted = true,
            omitted_reason = "response token budget exhausted (~" .. MAX_TOTAL_TEXT_TOKENS ..
                " tokens/call). Continue from next_offset in a follow-up call, view fewer files at once, or select a narrower page/artifact.",
        }
    end

    local chunk = content:sub(offset + 1, offset + limit)
    local next_offset = offset + #chunk
    local truncated = next_offset < total_size

    return {
        content = chunk,
        total_size = total_size,
        content_offset = offset,
        content_limit = requested_limit,
        content_length = #chunk,
        content_truncated = truncated,
        next_offset = truncated and next_offset or nil,
    }
end

local function apply_text_window(result: any, args: any, content: string, remaining_tokens: integer): integer
    local window = read_window(args, content, remaining_tokens)
    for key, value in pairs(window) do
        result[key] = value
    end
    local used_bytes: integer = math.floor(tonumber(window.content_length or 0) or 0)
    return remaining_tokens - est_tokens(used_bytes)
end

-- Raw bytes must never reach the agent: JSON-escaped binary (e.g. \0) explodes
-- the token count and is unreadable, which kills the turn. Treat content as binary
-- when it carries NUL bytes or a high ratio of non-text control bytes in a sample.
local function looks_binary(content: string): boolean
    if type(content) ~= "string" or content == "" then return false end
    local sample = content:sub(1, 2048)
    if sample:find("\0", 1, true) then return true end
    local nonprint = 0
    for i = 1, #sample do
        local b = sample:byte(i)
        if b ~= 9 and b ~= 10 and b ~= 13 and (b < 32 or b == 127) then
            nonprint = nonprint + 1
        end
    end
    return (nonprint / #sample) > 0.10
end

-- Present file content safely: text gets a token-bounded window; binary is never
-- dumped -- it returns a short, actionable note so the agent reads it the right way
-- (extracted text/images via artifacts) instead of choking on raw bytes.
local function apply_content(result: any, args: any, content: string, content_type: string, remaining_tokens: integer): integer
    if looks_binary(content) then
        result.total_size = #content
        result.content_omitted = true
        result.binary = true
        result.omitted_reason = "binary file (" .. (content_type ~= "" and content_type or "unknown type") .. ", " ..
            #content .. " bytes) -- not shown as text. If this is a document (PDF/DOCX/etc.), read its extracted " ..
            "text/images via artifact_refs, not the raw upload; raw bytes are not readable."
        return remaining_tokens
    end
    return apply_text_window(result, args, content, remaining_tokens)
end

local function apply_image_content(result: any, content: string, image_budget: any): boolean
    result.total_size = #content

    if #content > MAX_INLINE_IMAGE_BYTES then
        result.image_omitted = true
        result.omitted_reason = "image is too large to inline safely; use the upload/artifact preview URL in the UI"
        return false
    end

    if image_budget.used + #content > MAX_INLINE_IMAGE_BYTES_TOTAL then
        result.image_omitted = true
        result.omitted_reason = "per-call image budget exhausted; request fewer artifacts"
        return false
    end

    result.content = base64.encode(content) or content
    image_budget.used = image_budget.used + #content
    return true
end

local function parse_page(value: any): integer?
    local n = tonumber(value)
    if n and n >= 0 then return math.floor(n) end
    if type(value) == "string" then
        local found = value:match("[Pp]age%s*(%d+)")
        if found then return math.floor(tonumber(found) or 0) end
    end
    return nil
end

local function resolve_artifact_ref(upload_id: string, ref: any): (any?, string?)
    if type(ref) ~= "table" then return nil, "artifact reference must be an object" end

    local candidates: { any } = {}
    local artifact_id = trim(ref.artifact_id)
    if artifact_id ~= "" then
        local artifact = artifact_repo.get_artifact(upload_id, artifact_id)
        if artifact then
            return { artifact }, nil
        end
        candidates[#candidates + 1] = artifact_id
    end

    for _, key in ipairs({ "filename", "label", "name" }) do
        local value = trim(ref[key])
        if value ~= "" then candidates[#candidates + 1] = value end
    end

    local seen: { [string]: boolean } = {}
    for _, label in ipairs(candidates) do
        if not seen[label] then
            seen[label] = true
            local found, find_err = artifact_repo.find_artifact_parts(upload_id, {
                label = label,
                limit = artifact_ref_limit(ref),
                images_only = true,
            })
            if find_err then return nil, find_err end
            if found and #found > 0 then return found, nil end
        end
    end

    local page = parse_page(ref.page or ref.selector or ref.query)
    if page then
        local found, find_err = artifact_repo.find_artifact_parts(upload_id, {
            page = page,
            limit = artifact_ref_limit(ref),
            images_only = true,
        })
        if find_err then return nil, find_err end
        if found and #found > 0 then return found, nil end
    end

    return nil, "artifact not found"
end

-- Markdown is the agent-facing format: far fewer tokens than escaped JSON/XML and
-- directly readable. Each file is a heading + a fenced text window (or a one-line
-- note for binary/omitted/errored content), plus any image-artifact index.
local function build_markdown_response(results)
    local parts: { string } = {}

    for _, result in ipairs(results) do
        local header = "### " .. tostring(result.filename or "unknown") .. "  `" .. tostring(result.content_type or "unknown") .. "`"
        if result.total_size then header = header .. " · " .. tostring(result.total_size) .. " bytes" end
        table.insert(parts, header)

        if result.error then
            table.insert(parts, "> error: " .. tostring(result.error))
        elseif result.is_image then
            table.insert(parts, result.content and "_(image attached below)_" or ("> " .. tostring(result.omitted_reason or "image omitted")))
        elseif result.content then
            local off = tonumber(result.content_offset) or 0
            local len = tonumber(result.content_length) or #result.content
            local meta = "bytes " .. off .. "-" .. (off + len) .. " of " .. (tonumber(result.total_size) or len)
            if result.content_truncated then
                meta = meta .. " (truncated; continue with offset=" .. (tonumber(result.next_offset) or (off + len)) .. ")"
            end
            table.insert(parts, "_" .. meta .. "_")
            table.insert(parts, "```\n" .. result.content .. "\n```")
        elseif result.content_omitted then
            table.insert(parts, "> " .. tostring(result.omitted_reason or "content omitted"))
        end

        if result.image_artifacts and #result.image_artifacts > 0 then
            local total = tonumber(result.artifact_parts_total) or #result.image_artifacts
            table.insert(parts, "**" .. #result.image_artifacts .. " image(s)** (of " .. total .. ") -- view with `artifact_refs`:")
            for _, im in ipairs(result.image_artifacts) do
                local dims = (im.width and im.height) and (" " .. tostring(im.width) .. "x" .. tostring(im.height)) or ""
                table.insert(parts, "- " .. tostring(im.filename or im.artifact_id or "image") .. dims .. "  `artifact_id=" .. tostring(im.artifact_id) .. "`")
            end
        end

        table.insert(parts, "")
    end

    return table.concat(parts, "\n")
end

local function handle(args)
    args = args or {}

    local actor = security.actor()
    if not actor then
        return "Error: Authentication required to view files"
    end

    local upload_ids = type(args.upload_ids) == "table" and args.upload_ids or {}
    local artifact_refs = type(args.artifact_refs) == "table" and args.artifact_refs or {}

    if #upload_ids == 0 and #artifact_refs == 0 then
        return "Error: At least one upload ID or artifact reference is required"
    end

    if #upload_ids > 10 or #artifact_refs > 10 then
        return "Error: Maximum 10 files or artifacts can be viewed at once"
    end

    local content_contract, err = contract.get("kickside.contract:content_provider")
    if err then
        return "Error: Failed to get content provider contract: " .. err
    end

    local results = {}
    local has_images = false
    local has_artifacts = false
    local has_text = false
    local uses_json = false
    local remaining_text = MAX_TOTAL_TEXT_TOKENS
    local image_budget = { used = 0 }

    local function open_content(upload_id: string): (any?, string?)
        return content_contract
            :with_context({ upload_id = upload_id })
            :open("kickside.uploads:content_provider")
    end

    local selected_artifacts = 0
    for _, ref in ipairs(artifact_refs) do
        local upload_id = type(ref) == "table" and tostring(ref.upload_id or "") or ""
        if upload_id == "" then
            table.insert(results, { error = "artifact_refs entries require upload_id" })
        else
            local instance, err = open_content(upload_id)
            if err then
                table.insert(results, { upload_id = upload_id, error = "Failed to access file: " .. err })
            else
                local info, info_err = instance:get_info()
                if info_err then
                    table.insert(results, { upload_id = upload_id, error = "Failed to get file info: " .. info_err })
                else
                    local artifacts, artifact_err = resolve_artifact_ref(upload_id, ref)
                    if artifact_err or not artifacts or #artifacts == 0 then
                        table.insert(results, { upload_id = upload_id, error = "Failed to get artifact: " .. tostring(artifact_err) })
                    else
                        for _, artifact in ipairs(artifacts) do
                            if selected_artifacts >= MAX_SELECTED_ARTIFACTS then
                                table.insert(results, { upload_id = upload_id, error = "Selected artifact limit reached; request fewer artifacts" })
                                break
                            end
                            selected_artifacts = selected_artifacts + 1
                            local artifact_id = tostring(artifact.artifact_id or "")
                            local content, content_err = artifact_repo.read_artifact_content(upload_id, artifact_id)
                            if content_err or type(content) ~= "string" then
                                table.insert(results, { upload_id = upload_id, artifact_id = artifact_id, error = "Failed to read artifact: " .. tostring(content_err) })
                            else
                                local content_type = artifact.mime_type or "application/octet-stream"
                                local is_image = is_image_type(content_type)
                                if is_image then uses_json = true else has_text = true end
                                local result = {
                                    upload_id = upload_id,
                                    artifact_id = artifact_id,
                                    filename = artifact.label or (info and info.filename) or "artifact",
                                    content_type = content_type,
                                    is_image = is_image,
                                }
                                if is_image then
                                    if apply_image_content(result, content, image_budget) then has_images = true end
                                else
                                    remaining_text = apply_content(result, args, content, content_type, remaining_text)
                                end
                                table.insert(results, result)
                            end
                        end
                    end
                end
            end
        end
    end

    for _, raw_upload_id in ipairs(upload_ids) do
        local upload_id: string = tostring(raw_upload_id or "")
        if upload_id == "" then
            table.insert(results, {
                upload_id = upload_id,
                error = "upload_ids entries must be non-empty strings"
            })
        else
            local instance, err = open_content(upload_id)

            if err then
                table.insert(results, {
                    upload_id = upload_id,
                    error = "Failed to access file: " .. err
                })
            else
                local info: any, err = instance:get_info()
                if err then
                    table.insert(results, {
                        upload_id = upload_id,
                        error = "Failed to get file info: " .. err
                    })
                else
                    local is_image = is_image_type(info.content_type)

                    if is_image then
                        uses_json = true
                    else
                        has_text = true
                    end

                    local content_result, content_err = instance:get_content()
                    if content_err or not content_result or type(content_result.content) ~= "string" then
                        table.insert(results, {
                            upload_id = upload_id,
                            filename = info.filename,
                            content_type = info.content_type,
                            total_size = info.size,
                            error = "Failed to get content: " .. tostring(content_err or "content provider returned no content")
                        })
                    elseif is_image then
                        local result = {
                            upload_id = upload_id,
                            filename = info.filename or "unknown",
                            content_type = content_result.content_type or info.content_type,
                            is_image = true
                        }
                        if apply_image_content(result, content_result.content, image_budget) then has_images = true end
                        table.insert(results, result)
                    else
                        local image_artifacts, artifact_total, artifact_err = list_image_artifacts(tostring(upload_id), args)
                        if image_artifacts and #image_artifacts > 0 then has_artifacts = true; uses_json = true end
                        local result = {
                            upload_id = upload_id,
                            filename = info.filename or "unknown",
                            content_type = content_result.content_type or info.content_type,
                            is_image = false,
                            image_artifacts = image_artifacts,
                            artifact_parts_total = artifact_total,
                            artifact_offset = clamp_int(args.artifact_offset, 0, 0, 1000000000),
                            artifact_limit = clamp_int(args.artifact_limit, DEFAULT_ARTIFACT_LIST_LIMIT, 1, MAX_ARTIFACT_LIST_LIMIT),
                            artifact_error = artifact_err,
                        }
                        remaining_text = apply_content(result, args, content_result.content, tostring(content_result.content_type or info.content_type or ""), remaining_text)
                        table.insert(results, result)
                    end
                end
            end
        end
    end

    -- Markdown for everything the agent reads; inline images must still ride in the
    -- structured _images field for vision, so attach those alongside the markdown.
    local inline_images: { any } = {}
    for _, result in ipairs(results) do
        if result.is_image and result.content then
            inline_images[#inline_images + 1] = {
                type = "image",
                source = { type = "base64", mime_type = result.content_type or "application/octet-stream", data = result.content },
            }
        end
    end

    local markdown = build_markdown_response(results)
    if #inline_images > 0 then
        return json.encode({ result = markdown, _images = inline_images })
    end
    return markdown
end

return { handle = handle }

