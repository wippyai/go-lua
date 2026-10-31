local artifact_repo = require("artifact_repo")

-- Single source of truth for "what is a page image" of a document, shared by the
-- rasterize stage (which decides whether to skip rendering) and the OCR stage (which
-- decides what to transcribe). Keeping both on the same definition prevents the two
-- decisions from diverging.
local M = {}

M.DEFAULT_MIN_PAGE_BYTES = 51200
M.DEFAULT_MAX_TEXT_BYTES_PER_PAGE = 800
M.DEFAULT_CONFIDENT_TEXT_BYTES_PER_PAGE = 1600
M.DEFAULT_MIN_TEXT_SOURCE_RATIO = 0.02

local function trim(s: any): string
    if type(s) ~= "string" then return "" end
    return (s :: string):match("^%s*(.-)%s*$") or ""
end

local function sorted_by_ordinal(list: { any }): { any }
    table.sort(list, function(a, b) return (tonumber(a.ordinal) or 0) < (tonumber(b.ordinal) or 0) end)
    return list :: { any }
end

-- Rendered-page screenshots produced by the rasterizer.
function M.rendered(upload_id: string): { any }
    local artifacts = artifact_repo.list_artifacts(upload_id) or {}
    local out = {}
    for _, a in ipairs(artifacts) do
        if a.kind == "image" and a.role == "rendered-page" then out[#out + 1] = a end
    end
    return sorted_by_ordinal(out)
end

-- doc2md-extracted images large enough to be full pages (a page scan, not a logo or
-- inline figure). Byte size is the page-size proxy: a scanned page is substantial,
-- decorations are not. When in doubt the set is smaller, so the rasterizer renders.
function M.extracted_pages(upload_id: string, min_bytes: integer): { any }
    local artifacts = artifact_repo.list_artifacts(upload_id) or {}
    local out = {}
    for _, a in ipairs(artifacts) do
        if a.kind == "image" and a.role == "extracted-image" and (tonumber(a.size) or 0) >= min_bytes then
            out[#out + 1] = a
        end
    end
    return sorted_by_ordinal(out)
end

-- The page-image set to OCR/embed: rendered screenshots when present, otherwise the
-- large extracted images. Returns (list, source) where source is "rendered"|"extracted".
function M.page_set(upload_id: string, min_bytes: integer): ({ any }, string)
    local rendered = M.rendered(upload_id)
    if #rendered > 0 then return rendered, "rendered" end
    return M.extracted_pages(upload_id, min_bytes), "extracted"
end

function M.needs_ocr_text(content: any, page_count: any, max_text_per_page: any): boolean
    return M.ocr_text_reason(content, page_count, max_text_per_page) ~= nil
end

function M.ocr_text_reason(content: any, page_count: any, max_text_per_page: any): string?
    local text = trim(content)
    if text == "" then return "empty_text" end

    local pages = math.max(1, math.floor(tonumber(page_count) or 1))
    local per_page = math.max(1, math.floor(tonumber(max_text_per_page) or M.DEFAULT_MAX_TEXT_BYTES_PER_PAGE))
    if #text <= (pages * per_page) then return "sparse_text_per_page" end

    if pages > 1 then
        local confident_per_page = math.max(per_page + 1, M.DEFAULT_CONFIDENT_TEXT_BYTES_PER_PAGE)
        if #text < (pages * confident_per_page) then return "text_does_not_cover_pages" end
    end

    return nil
end

function M.needs_ocr_document(content: any, page_count: any, max_text_per_page: any, source_size: any): boolean
    return M.ocr_document_reason(content, page_count, max_text_per_page, source_size) ~= nil
end

function M.ocr_document_reason(content: any, page_count: any, max_text_per_page: any, source_size: any): string?
    local text_reason = M.ocr_text_reason(content, page_count, max_text_per_page)
    if text_reason then return text_reason end

    local size = tonumber(source_size) or 0
    if size < M.DEFAULT_MIN_PAGE_BYTES then return nil end

    local text = trim(content)
    if text == "" then return "empty_text" end

    local pages = math.max(1, math.floor(tonumber(page_count) or 1))
    local confident_per_page = math.max(
        math.floor(tonumber(max_text_per_page) or M.DEFAULT_MAX_TEXT_BYTES_PER_PAGE) + 1,
        M.DEFAULT_CONFIDENT_TEXT_BYTES_PER_PAGE
    )

    if (#text / size) < M.DEFAULT_MIN_TEXT_SOURCE_RATIO and #text < (pages * confident_per_page * 2) then
        return "low_text_to_source_ratio"
    end

    return nil
end

return M

