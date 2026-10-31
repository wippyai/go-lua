local time = require("time")
local metadata_sanitizer = require("metadata_sanitizer")

-- Normalize old numeric timestamps and current RFC3339 strings for frontend use.
local function format_date(timestamp)
    if timestamp == nil then return nil end
    if type(timestamp) == "string" then
        if timestamp == "" then return nil end
        if tonumber(timestamp) == nil then return timestamp end
    end

    if type(timestamp) ~= "number" then
        timestamp = tonumber(timestamp)
    end

    if not timestamp then return nil end

    local date = time.unix(timestamp, 0)
    return date:format_rfc3339()
end

local function format_list_upload(upload: any): any
    local metadata = metadata_sanitizer.public(upload.metadata)
    return {
        uuid = upload.uuid,
        size = upload.size,
        mime_type = upload.mime_type,
        status = upload.status,
        error = upload.error,
        created_at = upload.created_at,
        updated_at = upload.updated_at,
        filename = metadata.filename,
        meta = metadata,
    }
end

local function format_get_upload(upload: any): any
    local metadata = metadata_sanitizer.public(upload.metadata)
    return {
        uuid = upload.uuid,
        size = upload.size,
        mime_type = upload.mime_type,
        status = upload.status,
        error = upload.error,
        created_at = format_date(upload.created_at),
        updated_at = format_date(upload.updated_at),
        meta = metadata,
    }
end

return {
    format_date = format_date,
    format_list_upload = format_list_upload,
    format_get_upload = format_get_upload,
}
