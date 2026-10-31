local json = require("json")
local time = require("time")
local uuid = require("uuid")
local hash = require("hash")
local logger = require("logger")
local sql = require("sql")
local errors = require("errors")

local resources = require("uploads_resources")
local artifact_schema = require("artifact_schema")

local log = logger:named("artifact_repo")

local artifact_repo = {}
local collect_orphan_blobs: any

local function typed(kind: any, message: string): any
    return errors.new({ message = message, kind = kind })
end

local function now()
    return time.now():utc():format_rfc3339()
end

local function encode_json(value, fallback)
    value = value or fallback or {}
    if type(value) ~= "table" then return value end
    local encoded, err = json.encode(value)
    if err then return nil, "failed to encode json: " .. err end
    return encoded
end

local function decode_json(value)
    if value == nil or value == "" then return {} end
    local decoded, err = json.decode(tostring(value))
    if err or type(decoded) ~= "table" then return {} end
    return decoded
end

local function nullable(value)
    if value == nil or value == "" then return sql.as.null() end
    return value
end

local function digest_for(content)
    local digest, err = hash.sha256(content or "")
    if err then return nil, err end
    if not digest or digest == "" then return nil, "empty digest" end
    return digest
end

-- Each attempted blob owns a distinct storage path. Deduplication still happens on
-- (storage_id, checksum) in the database, but a losing concurrent writer can now
-- remove only the bytes it wrote without risking the winning blob's content.
local function artifact_path(checksum: string, blob_id: string, requested_path: string?): string
    if type(requested_path) == "string" and requested_path ~= "" then
        return requested_path .. ".artifact-" .. blob_id
    end
    return "artifact-sha256-" .. checksum .. "-" .. blob_id
end

local function remove_blob_bytes(storage, storage_path: string): string?
    local ok, _removed, remove_err = pcall(function() return storage:remove(storage_path) end)
    if not ok then
        return "artifact blob cleanup raised: " .. tostring(_removed)
    end
    if remove_err ~= nil and not tostring(remove_err):find("no such file or directory", 1, true) then
        return "artifact blob cleanup failed: " .. tostring(remove_err)
    end
    return nil
end

local function load_blob_by_checksum(db, storage_id, checksum)
    local query = sql.builder.select(
            "blob_id", "checksum", "mime_type", "size", "storage_id",
            "storage_path", "metadata", "created_at", "updated_at"
        )
        :from("kickside_upload_artifact_blobs")
        :where("storage_id = ? AND checksum = ?", storage_id, checksum)
        :limit(1)

    local rows, err = query:run_with(db):query()
    if err then return nil, "failed to load artifact blob: " .. err end
    if #rows == 0 then return nil end
    local row = rows[1]
    row.metadata = decode_json(row.metadata)
    return row
end

local function load_blob_by_id(db, blob_id)
    local query = sql.builder.select(
            "blob_id", "checksum", "mime_type", "size", "storage_id",
            "storage_path", "metadata", "created_at", "updated_at"
        )
        :from("kickside_upload_artifact_blobs")
        :where("blob_id = ?", blob_id)
        :limit(1)

    local rows, err = query:run_with(db):query()
    if err then return nil, "failed to load artifact blob: " .. err end
    if #rows == 0 then return nil, typed(errors.NOT_FOUND, "artifact blob not found") end
    local row = rows[1]
    row.metadata = decode_json(row.metadata)
    return row
end

local function normalize_artifact_row(row)
    if not row then return row end
    row.locator = decode_json(row.locator)
    row.metadata = decode_json(row.metadata)
    if row.occurrence_count ~= nil then
        row.occurrence_count = tonumber(row.occurrence_count) or 1
        row.duplicate_count = math.max(0, row.occurrence_count - 1)
    end
    return row
end

local function load_artifact_by_id(db, artifact_id)
    local rows, err = sql.builder.select(
            "a.artifact_id", "a.upload_id", "a.parent_artifact_id", "a.blob_id", "a.checksum", "a.kind",
            "a.role", "a.mime_type", "a.label", "a.ordinal", "a.locator", "a.metadata",
            "a.created_at", "a.updated_at", "b.storage_id", "b.storage_path", "b.size"
        )
        :from("kickside_upload_artifacts a")
        :left_join("kickside_upload_artifact_blobs b ON a.blob_id = b.blob_id")
        :where("a.artifact_id = ?", artifact_id)
        :limit(1)
        :run_with(db)
        :query()
    if err then return nil, "failed to load artifact: " .. err end
    if #rows == 0 then return nil, typed(errors.NOT_FOUND, "artifact not found") end
    return normalize_artifact_row(rows[1])
end

local function locator_storage(locator)
    if type(locator) ~= "table" then return nil, nil end
    local storage_id = locator.storage_id
    local storage_path = locator.storage_path
    if type(storage_id) ~= "string" or storage_id == "" then return nil, nil end
    if type(storage_path) ~= "string" or storage_path == "" then return nil, nil end
    return storage_id, storage_path
end

local function is_primary_text_artifact(artifact)
    if not artifact or artifact.role ~= "primary" then return false end
    local kind = tostring(artifact.kind or "")
    local mime_type = tostring(artifact.mime_type or ""):lower()
    if kind == "markdown" or kind == "text" then return true end
    if mime_type == "text/markdown" or mime_type:match("^text/") then return true end
    return mime_type == "application/json"
        or mime_type == "application/xml"
        or mime_type == "application/javascript"
        or mime_type == "application/sql"
        or mime_type == "application/xhtml+xml"
end

local function prefix_upper_bound(prefix)
    if type(prefix) ~= "string" or prefix == "" then return nil end
    local last = prefix:byte(#prefix)
    if not last or last >= 255 then return nil end
    return prefix:sub(1, #prefix - 1) .. string.char(last + 1)
end

local function artifact_part_key(row)
    if not row then return "" end
    if type(row.blob_id) == "string" and row.blob_id ~= "" then return "blob:" .. row.blob_id end
    if type(row.checksum) == "string" and row.checksum ~= "" then return "sha256:" .. row.checksum end
    return "artifact:" .. tostring(row.artifact_id or "")
end

local function add_occurrence_sample(row, occurrence)
    local label = occurrence and occurrence.label
    if type(label) ~= "string" or label == "" then return end
    row.occurrence_labels = row.occurrence_labels or {}
    if #row.occurrence_labels >= 8 then return end
    for _, existing in ipairs(row.occurrence_labels) do
        if existing == label then return end
    end
    row.occurrence_labels[#row.occurrence_labels + 1] = label
end

local function dedupe_artifact_part_rows(rows)
    local unique = {}
    local by_key = {}
    for _, row in ipairs(rows or {}) do
        local key = artifact_part_key(row)
        local representative = by_key[key]
        if representative then
            local occurrence_count = (tonumber(representative.occurrence_count) or 1) + 1
            representative.occurrence_count = occurrence_count
            representative.duplicate_count = occurrence_count - 1
            add_occurrence_sample(representative, row)
        else
            row.dedupe_key = key
            row.representative_artifact_id = row.artifact_id
            row.occurrence_count = 1
            row.duplicate_count = 0
            add_occurrence_sample(row, row)
            by_key[key] = row
            unique[#unique + 1] = row
        end
    end
    return unique
end

local function slice_rows(rows, limit, offset)
    rows = rows or {}
    offset = math.max(0, math.floor(tonumber(offset) or 0))
    limit = tonumber(limit)
    if not limit or limit <= 0 then
        limit = #rows
    else
        limit = math.floor(limit)
    end

    local out = {}
    local first = offset + 1
    local last = math.min(#rows, offset + limit)
    for i = first, last do
        out[#out + 1] = rows[i]
    end
    return out
end

function artifact_repo.create_blob(mime_type, content, metadata, options)
    if not mime_type or mime_type == "" then return nil, "mime_type is required" end
    if type(content) ~= "string" then return nil, "content must be a binary string" end
    options = options or {}

    local checksum, checksum_err = digest_for(content)
    if checksum_err then return nil, checksum_err end

    -- Encode every fallible in-memory field before mutating external storage.
    local metadata_json, encode_err = encode_json(metadata)
    if encode_err then return nil, encode_err end

    local storage_id = resources.get_storage_id(options.storage_id)

    local db, err = resources.get_db()
    if err then return nil, err end

    local existing, load_err = load_blob_by_checksum(db, storage_id, checksum)
    if load_err then
        db:release()
        return nil, load_err
    end
    if existing then
        db:release()
        return existing
    end

    local blob_id = uuid.v4()
    local requested_path: string? = nil
    if type(options.storage_path) == "string" then
        requested_path = options.storage_path :: string
    end
    local storage_path = artifact_path(checksum, blob_id, requested_path)

    local storage, storage_err = resources.get_storage(storage_id)
    if storage_err or not storage then
        db:release()
        return nil, storage_err or "failed to get artifact storage"
    end

    local write_ok, write_err = storage:writefile(storage_path, content)
    if write_ok == false or write_err then
        db:release()
        local cleanup_err = remove_blob_bytes(storage, storage_path)
        if cleanup_err then
            return nil, "failed to write artifact blob: " .. tostring(write_err) .. "; " .. cleanup_err
        end
        return nil, "failed to write artifact blob: " .. tostring(write_err)
    end

    local timestamp = now()
    local query = sql.builder.insert("kickside_upload_artifact_blobs")
        :set_map({
            blob_id = blob_id,
            checksum = checksum,
            mime_type = mime_type,
            size = sql.as.int(#content),
            storage_id = storage_id,
            storage_path = storage_path,
            metadata = metadata_json,
            created_at = timestamp,
            updated_at = timestamp,
        })

    local _, insert_err = query:run_with(db):exec()
    if insert_err then
        local raced, raced_err = load_blob_by_checksum(db, storage_id, checksum)
        db:release()
        local cleanup_err = remove_blob_bytes(storage, storage_path)
        if cleanup_err then
            return nil, "failed to create artifact blob: " .. insert_err .. "; " .. cleanup_err
        end
        if raced_err then
            return nil, "failed to create artifact blob: " .. insert_err .. "; " .. raced_err
        end
        if raced then return raced end
        return nil, "failed to create artifact blob: " .. insert_err
    end

    db:release()
    return {
        blob_id = blob_id,
        checksum = checksum,
        mime_type = mime_type,
        size = #content,
        storage_id = storage_id,
        storage_path = storage_path,
        metadata = metadata or {},
        created_at = timestamp,
        updated_at = timestamp,
    }
end

function artifact_repo.create_artifact(args)
    args = args or {}
    local schema_ok, schema_err = artifact_schema.validate_artifact_args(args)
    if not schema_ok then return nil, schema_err end

    local blob_id = args.blob_id
    local blob
    if args.content ~= nil then
        local blob_err
        blob, blob_err = artifact_repo.create_blob(args.mime_type or "application/octet-stream", args.content, args.blob_metadata, {
            storage_id = args.storage_id,
            storage_path = args.storage_path,
        })
        if blob_err then return nil, blob_err end
        if not blob then return nil, "failed to create artifact blob" end
        blob_id = blob.blob_id
    end

    local db, err = resources.get_db()
    if err then return nil, err end

    if args.parent_artifact_id ~= nil and args.parent_artifact_id ~= "" then
        local parent, parent_err = load_artifact_by_id(db, args.parent_artifact_id)
        if parent_err then
            db:release()
            return nil, parent_err
        end
        if parent.upload_id ~= args.upload_id then
            db:release()
            return nil, "parent_artifact_id belongs to a different upload"
        end
    end

    if blob_id ~= nil and blob_id ~= "" and not blob then
        local blob_err
        blob, blob_err = load_blob_by_id(db, blob_id)
        if blob_err then
            db:release()
            return nil, blob_err
        end
    end

    local metadata_json, metadata_err = encode_json(args.metadata or {})
    if metadata_err then
        db:release()
        return nil, metadata_err
    end
    local locator_json, locator_err = encode_json(args.locator or {})
    if locator_err then
        db:release()
        return nil, locator_err
    end

    local artifact_id = args.artifact_id or uuid.v4()
    local timestamp = now()
    local mime_type = args.mime_type or (blob and blob.mime_type) or "application/octet-stream"
    local checksum = args.checksum or (blob and blob.checksum)
    local query = sql.builder.insert("kickside_upload_artifacts")
        :set_map({
            artifact_id = artifact_id,
            upload_id = args.upload_id,
            parent_artifact_id = nullable(args.parent_artifact_id),
            blob_id = nullable(blob_id),
            checksum = nullable(checksum),
            kind = args.kind,
            role = nullable(args.role),
            mime_type = mime_type,
            label = nullable(args.label),
            ordinal = sql.as.int(tonumber(args.ordinal) or 0),
            locator = locator_json,
            metadata = metadata_json,
            created_at = timestamp,
            updated_at = timestamp,
        })

    local _, insert_err = query:run_with(db):exec()
    db:release()
    if insert_err then return nil, "failed to create artifact: " .. insert_err end

    return {
        artifact_id = artifact_id,
        upload_id = args.upload_id,
        parent_artifact_id = args.parent_artifact_id,
        blob_id = blob_id,
        checksum = checksum,
        kind = args.kind,
        role = args.role,
        mime_type = mime_type,
        label = args.label,
        ordinal = tonumber(args.ordinal) or 0,
        locator = args.locator or {},
        metadata = args.metadata or {},
        created_at = timestamp,
        updated_at = timestamp,
    }
end

-- Replace an artifact's content: stores a new (dedup-aware) blob and repoints the
-- artifact row at it. Used when a later pipeline stage supersedes an earlier stage's
-- output -- e.g. OCR replacing an empty doc2md markdown primary with transcribed text.
function artifact_repo.update_content(upload_id, artifact_id, content, options)
    if type(content) ~= "string" then return nil, "content must be a string" end
    options = options or {}

    local artifact, get_err = artifact_repo.get_artifact(upload_id, artifact_id)
    if get_err then return nil, get_err end
    if not artifact then return nil, typed(errors.NOT_FOUND, "artifact not found") end

    local mime_type = options.mime_type or artifact.mime_type or "application/octet-stream"

    local storage_id = options.storage_id
    if (not storage_id or storage_id == "") and artifact.blob_id and artifact.blob_id ~= "" then
        local existing_blob = artifact_repo.get_blob(artifact.blob_id)
        if existing_blob then storage_id = existing_blob.storage_id end
    end

    local blob, blob_err = artifact_repo.create_blob(mime_type, content, options.blob_metadata, { storage_id = storage_id })
    if blob_err then return nil, blob_err end
    if not blob then return nil, "failed to create artifact blob" end

    local db, err = resources.get_db()
    if err then return nil, err end

    local timestamp = now()
    local _, update_err = sql.builder.update("kickside_upload_artifacts")
        :set_map({
            blob_id = blob.blob_id,
            checksum = blob.checksum,
            mime_type = mime_type,
            updated_at = timestamp,
        })
        :where("artifact_id = ?", artifact_id)
        :where("upload_id = ?", upload_id)
        :run_with(db)
        :exec()
    db:release()
    if update_err then return nil, "failed to update artifact content: " .. update_err end
    if type(artifact.blob_id) == "string" and artifact.blob_id ~= "" and artifact.blob_id ~= blob.blob_id then
        local cleanup_db, cleanup_db_err = resources.get_db()
        if cleanup_db_err then return nil, cleanup_db_err end
        local _, cleanup_err = (collect_orphan_blobs :: any)(cleanup_db, { artifact.blob_id })
        cleanup_db:release()
        if cleanup_err then return nil, cleanup_err end
    end

    return {
        artifact_id = artifact_id,
        upload_id = upload_id,
        blob_id = blob.blob_id,
        checksum = blob.checksum,
        mime_type = mime_type,
        size = blob.size,
        updated_at = timestamp,
    }
end

function artifact_repo.get_blob(blob_id)
    if not blob_id or blob_id == "" then return nil, "blob_id is required" end
    local db, err = resources.get_db()
    if err then return nil, err end
    local blob, blob_err = load_blob_by_id(db, blob_id)
    db:release()
    if blob_err then return nil, blob_err end
    return blob
end

function artifact_repo.get_artifact(upload_id, artifact_id)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    if not artifact_id or artifact_id == "" then return nil, "artifact_id is required" end

    local db, err = resources.get_db()
    if err then return nil, err end
    local artifact, artifact_err = load_artifact_by_id(db, artifact_id)
    db:release()
    if artifact_err then return nil, artifact_err end
    if artifact.upload_id ~= upload_id then return nil, typed(errors.NOT_FOUND, "artifact not found for upload") end
    return artifact
end

function artifact_repo.read_blob(blob_id)
    local blob, blob_err = artifact_repo.get_blob(blob_id)
    if blob_err then return nil, blob_err end
    if not blob then return nil, typed(errors.NOT_FOUND, "artifact blob not found") end
    if type(blob.storage_id) ~= "string" or type(blob.storage_path) ~= "string" then
        return nil, typed(errors.INVALID, "artifact blob storage location is invalid")
    end

    local storage_id = blob.storage_id :: string
    local storage_path = blob.storage_path :: string

    local storage, storage_err = resources.get_storage(storage_id)
    if storage_err or not storage then
        return nil, storage_err or "failed to get artifact storage"
    end

    local content = storage:readfile(storage_path)
    if type(content) ~= "string" then
        return nil, typed(errors.INVALID, "failed to read artifact blob")
    end
    return content, nil, blob
end

function artifact_repo.read_artifact_content(upload_id, artifact_id)
    local artifact, artifact_err = artifact_repo.get_artifact(upload_id, artifact_id)
    if artifact_err then return nil, artifact_err end
    if not artifact then return nil, typed(errors.NOT_FOUND, "artifact not found") end

    if artifact.blob_id and artifact.blob_id ~= "" then
        local content, content_err, blob = artifact_repo.read_blob(artifact.blob_id)
        if content_err then return nil, content_err end
        return content, nil, blob, artifact
    end

    local storage_id, storage_path = locator_storage(artifact.locator)
    if not storage_id or not storage_path then
        return nil, typed(errors.INVALID, "artifact has no readable bytes")
    end

    local artifact_storage_id = storage_id :: string
    local artifact_storage_path = storage_path :: string

    local storage, storage_err = resources.get_storage(artifact_storage_id)
    if storage_err or not storage then
        return nil, storage_err or "failed to get artifact storage"
    end

    local content = storage:readfile(artifact_storage_path)
    if type(content) ~= "string" then
        return nil, typed(errors.INVALID, "failed to read artifact bytes")
    end

    return content, nil, {
        blob_id = nil,
        checksum = artifact.checksum,
        mime_type = artifact.mime_type,
        size = #content,
        storage_id = artifact_storage_id,
        storage_path = artifact_storage_path,
        metadata = artifact.metadata or {},
    }, artifact
end

function artifact_repo.create_chunk(args)
    args = args or {}
    local schema_ok, schema_err = artifact_schema.validate_chunk_args(args)
    if not schema_ok then return nil, schema_err end

    local metadata_json, metadata_err = encode_json(args.metadata or {})
    if metadata_err then return nil, metadata_err end
    local locator_json, locator_err = encode_json(args.locator or {})
    if locator_err then return nil, locator_err end

    local db, err = resources.get_db()
    if err then return nil, err end

    if args.artifact_id ~= nil and args.artifact_id ~= "" then
        local artifact, artifact_err = load_artifact_by_id(db, args.artifact_id)
        if artifact_err then
            db:release()
            return nil, artifact_err
        end
        if artifact.upload_id ~= args.upload_id then
            db:release()
            return nil, "artifact_id belongs to a different upload"
        end
    end

    local chunk_id = args.chunk_id or uuid.v4()
    local timestamp = now()
    local query = sql.builder.insert("kickside_upload_chunks")
        :set_map({
            chunk_id = chunk_id,
            upload_id = args.upload_id,
            artifact_id = nullable(args.artifact_id),
            kind = args.kind,
            content = args.content,
            ordinal = sql.as.int(tonumber(args.ordinal) or 0),
            token_count = args.token_count and sql.as.int(args.token_count) or sql.as.null(),
            locator = locator_json,
            metadata = metadata_json,
            created_at = timestamp,
            updated_at = timestamp,
        })

    local _, insert_err = query:run_with(db):exec()
    db:release()
    if insert_err then return nil, "failed to create chunk: " .. insert_err end

    return {
        chunk_id = chunk_id,
        upload_id = args.upload_id,
        artifact_id = args.artifact_id,
        kind = args.kind,
        content = args.content,
        ordinal = tonumber(args.ordinal) or 0,
        token_count = args.token_count,
        locator = args.locator or {},
        metadata = args.metadata or {},
        created_at = timestamp,
        updated_at = timestamp,
    }
end

function artifact_repo.list_artifacts(upload_id)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    local db, err = resources.get_db()
    if err then return nil, err end

    local rows, query_err = sql.builder.select(
            "a.artifact_id", "a.upload_id", "a.parent_artifact_id", "a.blob_id", "a.checksum", "a.kind",
            "a.role", "a.mime_type", "a.label", "a.ordinal", "a.locator", "a.metadata",
            "a.created_at", "a.updated_at", "b.storage_id", "b.storage_path", "b.size"
        )
        :from("kickside_upload_artifacts a")
        :left_join("kickside_upload_artifact_blobs b ON a.blob_id = b.blob_id")
        :where("a.upload_id = ?", upload_id)
        :order_by("a.ordinal ASC, a.created_at ASC")
        :run_with(db)
        :query()

    db:release()
    if query_err then return nil, "failed to list artifacts: " .. query_err end
    for _, row in ipairs(rows) do
        normalize_artifact_row(row)
    end
    return rows
end

-- Extracted parts are every artifact except the original source and the primary
-- content: the images and other pieces a single upload can fan out into (hundreds
-- for a large document), surfaced as a paginated list rather than loaded at once.
local PARTS_WHERE = "a.role NOT IN ('source', 'primary')"

local function artifact_part_rows(upload_id, opts)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    opts = opts or {}
    local db, err = resources.get_db()
    if err then return nil, err end

    -- The prefix range trick requires byte-order comparison. SQLite's default
    -- BINARY collation gives that; postgres locale collations sort "page20_"
    -- inside the ["page2_", "page2`") range, so pin the C collation there.
    local label_expr = "a.label"
    if db:type() == sql.type.POSTGRES then
        label_expr = "a.label COLLATE \"C\""
    end

    local query = sql.builder.select(
            "a.artifact_id", "a.upload_id", "a.parent_artifact_id", "a.blob_id", "a.checksum", "a.kind",
            "a.role", "a.mime_type", "a.label", "a.ordinal", "a.locator", "a.metadata",
            "a.created_at", "a.updated_at", "b.storage_id", "b.storage_path", "b.size"
        )
        :from("kickside_upload_artifacts a")
        :left_join("kickside_upload_artifact_blobs b ON a.blob_id = b.blob_id")
        :where("a.upload_id = ?", upload_id)
        :where(PARTS_WHERE)

    local label = type(opts.label) == "string" and opts.label or ""
    if label ~= "" then
        query = query:where("(a.label = ? OR a.metadata LIKE ?)", label, "%" .. label .. "%")
    end

    local page = tonumber(opts.page)
    if page and page >= 0 then
        local prefix = "page" .. tostring(math.floor(page)) .. "_"
        local upper = prefix_upper_bound(prefix)
        if upper then
            query = query:where(label_expr .. " >= ? AND " .. label_expr .. " < ?", prefix, upper)
        else
            query = query:where("a.label LIKE ?", prefix .. "%")
        end
    end

    local prefix = type(opts.prefix) == "string" and opts.prefix or ""
    if prefix ~= "" then
        local upper = prefix_upper_bound(prefix)
        if upper then
            query = query:where(label_expr .. " >= ? AND " .. label_expr .. " < ?", prefix, upper)
        else
            query = query:where("a.label LIKE ?", prefix .. "%")
        end
    end

    if opts.images_only == true then
        query = query:where("(a.kind = 'image' OR a.mime_type LIKE 'image/%')")
    end

    local rows, query_err = query
        :order_by("a.ordinal ASC, a.created_at ASC, a.artifact_id ASC")
        :run_with(db)
        :query()
    db:release()
    if query_err then return nil, "failed to list artifact parts: " .. query_err end

    for _, row in ipairs(rows) do
        normalize_artifact_row(row)
    end
    if opts.include_duplicates == true then
        return rows
    end
    return dedupe_artifact_part_rows(rows)
end

function artifact_repo.count_artifact_parts(upload_id, opts)
    local rows, err = artifact_part_rows(upload_id, opts)
    if err then return nil, "failed to count artifact parts: " .. err end
    return #rows
end

function artifact_repo.list_artifact_parts(upload_id, limit, offset, opts)
    local rows, err = artifact_part_rows(upload_id, opts)
    if err then return nil, err end
    return slice_rows(rows, limit, offset)
end

function artifact_repo.find_artifact_parts(upload_id, opts)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    opts = opts or {}
    if opts.images_only == nil then opts.images_only = true end
    local limit = tonumber(opts.limit) or 10
    if limit < 1 then limit = 1 end
    if limit > 100 then limit = 100 end

    local rows, err = artifact_part_rows(upload_id, opts)
    if err then return nil, "failed to find artifact parts: " .. err end
    return slice_rows(rows, limit, 0)
end

function artifact_repo.get_primary_content_artifact(upload_id)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    local db, err = resources.get_db()
    if err then return nil, err end

    local rows, query_err = sql.builder.select(
            "a.artifact_id", "a.upload_id", "a.parent_artifact_id", "a.blob_id", "a.checksum", "a.kind",
            "a.role", "a.mime_type", "a.label", "a.ordinal", "a.locator", "a.metadata",
            "a.created_at", "a.updated_at", "b.storage_id", "b.storage_path", "b.size"
        )
        :from("kickside_upload_artifacts a")
        :left_join("kickside_upload_artifact_blobs b ON a.blob_id = b.blob_id")
        :where("a.upload_id = ?", upload_id)
        :where("a.role = 'primary'")
        :order_by("a.ordinal ASC, a.created_at ASC")
        :run_with(db)
        :query()

    db:release()
    if query_err then return nil, "failed to get primary content artifact: " .. query_err end
    for _, row in ipairs(rows) do
        normalize_artifact_row(row)
    end
    for _, artifact in ipairs(rows) do
        if is_primary_text_artifact(artifact) then
            return artifact
        end
    end
    return nil, typed(errors.INVALID, "primary content artifact not found")
end

function artifact_repo.read_primary_content(upload_id)
    local artifact, artifact_err = artifact_repo.get_primary_content_artifact(upload_id)
    if artifact_err then return nil, artifact_err end

    local content, content_err, blob = artifact_repo.read_artifact_content(upload_id, artifact.artifact_id)
    if content_err then return nil, content_err end
    return {
        content = content,
        mime_type = artifact.mime_type or (blob and blob.mime_type) or "text/markdown",
        metadata = artifact.metadata or {},
        artifact = artifact,
        blob = blob,
    }
end

function artifact_repo.list_chunks(upload_id)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    local db, err = resources.get_db()
    if err then return nil, err end

    local rows, query_err = sql.builder.select(
            "chunk_id", "upload_id", "artifact_id", "kind", "content", "ordinal",
            "token_count", "locator", "metadata", "created_at", "updated_at"
        )
        :from("kickside_upload_chunks")
        :where("upload_id = ?", upload_id)
        :order_by("ordinal ASC, created_at ASC")
        :run_with(db)
        :query()

    db:release()
    if query_err then return nil, "failed to list chunks: " .. query_err end
    for _, row in ipairs(rows) do
        row.locator = decode_json(row.locator)
        row.metadata = decode_json(row.metadata)
    end
    return rows
end

-- Reclaim every blob in candidate_ids that no surviving artifact still references, and
-- remove its storage bytes.
--
-- Blobs are content-addressed and shared across artifacts (deduplicated by storage_id +
-- checksum). A blob is reclaimed only when no kickside_upload_artifacts row points at it,
-- so a blob shared with a still-living artifact is kept; the candidate set bounds the
-- check to blobs that just lost a referencing artifact. Caller supplies an open db so the
-- reference check and the row delete observe the same post-delete state.
--
-- Storage bytes are removed before the blob row. If byte removal fails, the row stays
-- as a durable retry target for the next orphan collection pass.
collect_orphan_blobs = function(db, candidate_ids)
    local seen = {}
    local removed = 0
    for _, blob_id in ipairs(candidate_ids or {}) do
        if type(blob_id) == "string" and blob_id ~= "" and not seen[blob_id] then
            seen[blob_id] = true

            local refs, refs_err = sql.builder.select("a.artifact_id")
                :from("kickside_upload_artifacts a")
                :where("a.blob_id = ?", blob_id)
                :limit(1)
                :run_with(db)
                :query()
            if refs_err then return nil, "failed to check artifact blob references: " .. refs_err end
            if #refs == 0 then
                local blob, blob_err = load_blob_by_id(db, blob_id)
                if not blob_err and blob then
                    local storage, storage_err = resources.get_storage(blob.storage_id)
                    if storage_err or not storage then
                        return nil, "failed to get storage for orphan blob cleanup: " .. tostring(storage_err)
                    end
                    local ok, _removed, remove_err = pcall(function() return storage:remove(blob.storage_path :: string) end)
                    if not ok then
                        return nil, "failed to remove orphan blob bytes: " .. tostring(_removed)
                    end
                    if remove_err ~= nil then
                        if tostring(remove_err):find("no such file or directory", 1, true) then
                            remove_err = nil
                        end
                    end
                    if remove_err ~= nil then
                        return nil, "failed to remove orphan blob bytes: " .. tostring(remove_err)
                    end

                    local _, delete_err = sql.builder.delete("kickside_upload_artifact_blobs")
                        :where("blob_id = ?", blob_id)
                        :run_with(db)
                        :exec()
                    if delete_err then return nil, "failed to delete orphan artifact blob: " .. delete_err end
                    removed = removed + 1
                end
            end
        end
    end
    return removed
end

-- Delete an upload's artifacts and chunks, then reclaim any blob those artifacts held that
-- no other artifact still references.
--
-- The schema declares ON DELETE CASCADE/SET NULL, but SQLite does not enforce foreign keys
-- unless PRAGMA foreign_keys is enabled, so this removes the dependent rows explicitly
-- rather than relying on the upload-row cascade. Order matters: gather the candidate
-- blob_ids first, delete the artifacts (which drops the references), then collect the
-- blobs that are now orphaned.
function artifact_repo.delete_for_upload(upload_id)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    local db, err = resources.get_db()
    if err then return nil, err end

    local artifacts, artifacts_err = sql.builder.select("artifact_id", "blob_id")
        :from("kickside_upload_artifacts")
        :where("upload_id = ?", upload_id)
        :run_with(db)
        :query()
    if artifacts_err then
        db:release()
        return nil, "failed to list artifacts for delete: " .. artifacts_err
    end

    local candidate_ids = {}
    for _, row in ipairs(artifacts) do
        if type(row.blob_id) == "string" and row.blob_id ~= "" then
            candidate_ids[#candidate_ids + 1] = row.blob_id
        end
    end

    local _, chunks_err = sql.builder.delete("kickside_upload_chunks")
        :where("upload_id = ?", upload_id)
        :run_with(db)
        :exec()
    if chunks_err then
        db:release()
        return nil, "failed to delete upload chunks: " .. chunks_err
    end

    local _, delete_err = sql.builder.delete("kickside_upload_artifacts")
        :where("upload_id = ?", upload_id)
        :run_with(db)
        :exec()
    if delete_err then
        db:release()
        return nil, "failed to delete upload artifacts: " .. delete_err
    end

    local removed, collect_err = collect_orphan_blobs(db, candidate_ids)
    db:release()
    if collect_err then return nil, collect_err end
    return { removed = removed }
end

function artifact_repo.list_chunk_summaries(upload_id)
    if not upload_id or upload_id == "" then return nil, "upload_id is required" end
    local db, err = resources.get_db()
    if err then return nil, err end

    local rows, query_err = sql.builder.select(
            "chunk_id", "upload_id", "artifact_id", "kind", "ordinal",
            "token_count", "length(content) AS content_length", "locator", "metadata",
            "created_at", "updated_at"
        )
        :from("kickside_upload_chunks")
        :where("upload_id = ?", upload_id)
        :order_by("ordinal ASC, created_at ASC")
        :run_with(db)
        :query()

    db:release()
    if query_err then return nil, "failed to list chunk summaries: " .. query_err end
    for _, row in ipairs(rows) do
        row.locator = decode_json(row.locator)
        row.metadata = decode_json(row.metadata)
    end
    return rows
end

return artifact_repo

