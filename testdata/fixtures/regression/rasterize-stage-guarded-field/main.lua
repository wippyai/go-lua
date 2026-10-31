local funcs = require("funcs")
local contract = require("contract")
local logger = require("logger")
local artifact_repo = require("artifact_repo")
local rasterizer = require("rasterizer")
local page_images = require("page_images")

-- Universal upload pipeline stage. Runs after content extraction on every upload and
-- self-selects: it acts only when the source mime has a registered rasterizer AND the
-- extracted text is empty/below a threshold (a scanned or vector-only document). It
-- renders the pages into role="rendered-page" image artifacts so the OCR/caption path
-- can consume them. When doc2md already extracted one large image per page (the PDF's
-- pages ARE images), rendering is redundant and is skipped -- those extracted images
-- serve as the pages. Best-effort: a render failure is logged and the upload still
-- succeeds with its (empty) text.

local SETTINGS_CONTRACT = "kickside.settings:settings"
local SETTINGS_NS = "kickside.uploads.rasterize"

local log = logger:named("upload_rasterize")

local FORMAT_EXT: { [string]: string } = { png = "png", jpeg = "jpg", jpg = "jpg", webp = "webp" }

local function format_mime(fmt: string): string
    if fmt == "png" then return "image/png" end
    if fmt == "webp" then return "image/webp" end
    return "image/jpeg"
end

local function setting(key: string): any
    local def = contract.get(SETTINGS_CONTRACT)
    if not def then return nil end
    local inst = def:open()
    if not inst then return nil end
    local r = inst:get({ namespace = SETTINGS_NS, key = key })
    if r and r.success then return r.value end
    return nil
end

local function setting_int(key: string, default: integer): integer
    local v = tonumber(setting(key))
    if v == nil then return default end
    return math.floor(v) :: integer
end

local function setting_bool(key: string, default: boolean): boolean
    local v = setting(key)
    if v == nil or v == "" then return default end
    return v == true or tostring(v):lower() == "true"
end

local function setting_str(key: string, default: string): string
    local v = setting(key)
    if type(v) == "string" and (v :: string) ~= "" then return v :: string end
    return default
end

local function source_artifact_id(upload_id: string): string?
    local artifacts, err = artifact_repo.list_artifacts(upload_id)
    if err or type(artifacts) ~= "table" then return nil end
    for _, a in ipairs(artifacts) do
        if (type(a.role) == "string" and a.role == "source") or (type(a.kind) == "string" and a.kind == "file") then
            if type(a.artifact_id) == "string" and a.artifact_id ~= "" then return a.artifact_id end
        end
    end
    return nil
end

local function store_page(params: any, page: any, fmt: string, source_id: string?, markdown_id: string?): string?
    local page_no = tonumber(page.page) or 0
    local mime = format_mime(fmt)
    local name = "page_" .. tostring(page_no) .. "." .. (FORMAT_EXT[fmt] or "jpg")
    local _, err = artifact_repo.create_artifact({
        upload_id = params.upload_id,
        parent_artifact_id = source_id,
        kind = "image",
        role = "rendered-page",
        mime_type = mime,
        label = name,
        ordinal = 2000 + page_no,
        content = page.bytes,
        storage_id = params.storage_id,
        blob_metadata = {
            source = "rasterize",
            original_upload_id = params.upload_id,
            image_name = name,
            mime_type = mime,
            page = page_no,
            source_artifact_id = source_id,
            markdown_artifact_id = markdown_id,
        },
        locator = {
            type = "rasterize.page",
            page = page_no,
            source_artifact_id = source_id,
            markdown_artifact_id = markdown_id,
        },
        metadata = {
            source = "rasterize",
            name = name,
            mime_type = mime,
            page = page_no,
            original_upload_id = params.upload_id,
            parent_artifact_id = source_id,
            source_artifact_id = source_id,
            markdown_artifact_id = markdown_id,
        },
    })
    return err
end

local function render_pages(params: any, entry: any, ref: any, total: integer, opts: any, source_id: string?, markdown_id: string?): (integer, string?)
    local rendered = 0
    local page = 1
    while page <= total do
        local last = math.min((page + opts.batch - 1) :: number, total)
        local selector: { integer } = {}
        for p = page, last do selector[#selector + 1] = p end

        local res, call_err = funcs.call(entry.processor_func :: string, {
            ref = ref,
            dpi = opts.dpi,
            format = opts.format,
            quality = opts.quality,
            pages = selector,
        })
        if call_err ~= nil then return rendered, tostring(call_err) end
        if type(res) ~= "table" then return rendered, "rasterize: empty render result" end
        if res.err ~= nil then
            local msg = (type(res.err) == "table" and res.err.message) or tostring(res.err)
            return rendered, tostring(msg)
        end

        local ok = type(res.ok) == "table" and res.ok or {}
        for _, p in ipairs(ok.pages or {}) do
            local store_err = store_page(params, p, opts.format :: string, source_id, markdown_id)
            if store_err ~= nil then return rendered, tostring(store_err) end
            rendered = rendered + 1
        end

        page = last + 1
    end
    return rendered, nil
end

local function process(params: any): any
    -- A universal best-effort stage must never fail the upload: malformed params is a
    -- no-op, not a throw (which the pipeline would record as a failed upload).
    if type(params) ~= "table" or type(params.upload_id) ~= "string" then
        return { success = true }
    end
    local upload_id = params.upload_id :: string

    local mime = type(params.mime_type) == "string" and params.mime_type or ""

    -- Fast bail for the common case: nothing can rasterize this mime (non-PDF uploads).
    local entry = rasterizer.find_by_mime(mime)
    if not entry then return { success = true } end

    if not setting_bool("rasterize_empty_enabled", true) then return { success = true } end

    local min_source = setting_int("rasterize_min_source_bytes", 4096)
    if (tonumber(params.size) or 0) < min_source then return { success = true } end

    local max_text = setting_int("rasterize_max_text_bytes", page_images.DEFAULT_MAX_TEXT_BYTES_PER_PAGE)
    local primary, primary_err = artifact_repo.read_primary_content(upload_id)
    if primary_err or type(primary) ~= "table" then return { success = true } end

    local source_id = source_artifact_id(upload_id)
    local markdown_id = type(primary.artifact) == "table" and primary.artifact.artifact_id or nil

    local ref = { storage_id = params.storage_id, storage_path = params.storage_path }

    local count_res, count_err = funcs.call(entry.processor_func :: string, { ref = ref, count_only = true })
    if count_err ~= nil or type(count_res) ~= "table" or type(count_res.ok) ~= "table" then
        log:warn("rasterize page count failed", { upload_id = upload_id, error = tostring(count_err) })
        return { success = true }
    end
    local total = tonumber(count_res.ok.total) or 0
    if total < 1 then return { success = true } end
    local ocr_reason = page_images.ocr_document_reason(primary.content, total, max_text, params.size)
    if not ocr_reason then return { success = true } end

    -- Skip rendering when doc2md already extracted exactly one full-page image per page
    -- (the PDF's pages are images). Those extracted images are the pages -- re-rendering
    -- would duplicate them and discard the originals. Strict 1:1 match only; anything
    -- else (multi-image pages, logos, partial coverage) falls through to rendering.
    local min_page_bytes = setting_int("rasterize_min_page_bytes", page_images.DEFAULT_MIN_PAGE_BYTES)
    local extracted = page_images.extracted_pages(upload_id, min_page_bytes)
    if #extracted == total then
        log:info("pages already extracted by doc2md; skipping rasterize", {
            upload_id = upload_id, total_pages = total, extracted_pages = #extracted,
            reason = ocr_reason,
        })
        return { success = true, metadata = { rasterize_skipped = true } }
    end

    log:info("rasterizing document for OCR", {
        upload_id = upload_id,
        reason = ocr_reason,
        text_bytes = type(primary.content) == "string" and #(primary.content :: string) or 0,
        total_pages = total,
        extracted_page_images = #extracted,
    })

    local opts = {
        dpi = setting_int("rasterize_dpi", 150),
        batch = math.max(1, setting_int("rasterize_batch_pages", 8)),
        format = setting_str("rasterize_format", "jpeg"),
        quality = setting_int("rasterize_quality", 90),
    }

    local rendered, render_err = render_pages(params, entry, ref, total :: integer, opts, source_id, markdown_id)
    if render_err ~= nil then
        log:warn("rasterize render failed", { upload_id = upload_id, rendered = rendered, total = total, error = render_err })
        if rendered == 0 then return { success = true } end
    end

    log:info("rasterized empty document", {
        upload_id = upload_id, rendered_pages = rendered, total_pages = total,
        format = opts.format, rasterizer = entry.id, reason = ocr_reason,
    })

    return { success = true, metadata = { rendered_page_count = rendered, rasterize_reason = ocr_reason } }
end

return { process = process }

