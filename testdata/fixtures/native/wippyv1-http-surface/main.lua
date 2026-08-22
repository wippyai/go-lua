-- Contract: the Wippy v1 http module as the runtime declares it. Every
-- declared member of the module, of the Request and Response handles, and of
-- the MultipartFile and Stream handles the request hands out is called, each
-- fallible result is read on both of its arms, and every member that answers
-- an optional string is tested for the nil the module actually answers before
-- the value is used.

local http = require("http")

local methods: string = http.METHOD.GET .. http.METHOD.POST .. http.METHOD.PUT ..
    http.METHOD.DELETE .. http.METHOD.PATCH .. http.METHOD.HEAD .. http.METHOD.OPTIONS
local content: string = http.CONTENT.JSON .. http.CONTENT.FORM .. http.CONTENT.MULTIPART ..
    http.CONTENT.TEXT .. http.CONTENT.STREAM
local transfer: string = http.TRANSFER.CHUNKED .. http.TRANSFER.SSE
local ok_status: number = http.STATUS.OK + http.STATUS.CREATED + http.STATUS.ACCEPTED +
    http.STATUS.NO_CONTENT + http.STATUS.PARTIAL_CONTENT
local redirect_status: number = http.STATUS.MOVED_PERMANENTLY + http.STATUS.FOUND +
    http.STATUS.SEE_OTHER + http.STATUS.NOT_MODIFIED + http.STATUS.TEMPORARY_REDIRECT +
    http.STATUS.PERMANENT_REDIRECT
local client_status: number = http.STATUS.BAD_REQUEST + http.STATUS.UNAUTHORIZED +
    http.STATUS.PAYMENT_REQUIRED + http.STATUS.FORBIDDEN + http.STATUS.NOT_FOUND +
    http.STATUS.METHOD_NOT_ALLOWED + http.STATUS.NOT_ACCEPTABLE + http.STATUS.CONFLICT +
    http.STATUS.GONE + http.STATUS.UNPROCESSABLE + http.STATUS.TOO_MANY_REQUESTS
local server_status: number = http.STATUS.INTERNAL_ERROR + http.STATUS.INTERNAL_SERVER_ERROR +
    http.STATUS.NOT_IMPLEMENTED + http.STATUS.BAD_GATEWAY + http.STATUS.SERVICE_UNAVAILABLE +
    http.STATUS.GATEWAY_TIMEOUT + http.STATUS.VERSION_NOT_SUPPORTED

local request, request_err = http.request()
if request_err ~= nil then
    return request_err:kind()
end

local configured, configured_err = http.request({ timeout = 30 })
if configured_err ~= nil then
    return configured_err:message()
end

local response, response_err = http.response()
if response_err ~= nil then
    return response_err:message()
end

local verb, verb_err = request:method()
if verb_err ~= nil then
    return verb_err:message()
end

local path, path_err = configured:path()
if path_err ~= nil then
    return path_err:message()
end

-- query answers an optional string: the parameter is absent unless the caller
-- sent it, so the value is tested before it is used as a string.
local id, id_err = request:query("id")
if id_err ~= nil then
    return id_err:message()
end
local id_text: string = "anonymous"
if id ~= nil then
    id_text = id
end

local query_params, query_params_err = request:query_params()
if query_params_err ~= nil then
    return query_params_err:message()
end

local accept, accept_err = request:header("Accept")
if accept_err ~= nil then
    return accept_err:message()
end
local accept_text: string = "*/*"
if accept ~= nil then
    accept_text = accept
end

local media, media_err = request:content_type()
if media_err ~= nil then
    return media_err:message()
end
local media_text: string = http.CONTENT.TEXT
if media ~= nil then
    media_text = media
end

local length, length_err = request:content_length()
if length_err ~= nil then
    return length_err:message()
end

local host, host_err = request:host()
if host_err ~= nil then
    return host_err:message()
end

local remote, remote_err = request:remote_addr()
if remote_err ~= nil then
    return remote_err:message()
end

local body, body_err = request:body()
if body_err ~= nil then
    return body_err:message()
end

local document, document_err = request:body_json()
if document_err ~= nil then
    return document_err:message()
end

local has_body, has_body_err = request:has_body()
if has_body_err ~= nil then
    return has_body_err:message()
end

local accepts_json, accepts_json_err = request:accepts(http.CONTENT.JSON)
if accepts_json_err ~= nil then
    return accepts_json_err:message()
end

local is_json, is_json_err = request:is_content_type(http.CONTENT.JSON)
if is_json_err ~= nil then
    return is_json_err:message()
end

-- param answers an optional string for the same reason query does: a route
-- placeholder the request did not match has no value.
local slug, slug_err = request:param("slug")
if slug_err ~= nil then
    return slug_err:message()
end
local slug_text: string = "index"
if slug ~= nil then
    slug_text = slug
end

local params, params_err = request:params()
if params_err ~= nil then
    return params_err:message()
end

local stream, stream_err = request:stream()
if stream_err ~= nil then
    return stream_err:message()
end

local chunk, read_err = stream:read(1024)
if read_err ~= nil then
    return read_err:message()
end
local rest, rest_err = stream:read()
if rest_err ~= nil then
    return rest_err:message()
end
local written, write_err = stream:write(chunk)
if write_err ~= nil then
    return write_err:message()
end
local offset, seek_err = stream:seek("set", 0)
if seek_err ~= nil then
    return seek_err:message()
end
local rewound, rewind_err = stream:seek()
if rewind_err ~= nil then
    return rewind_err:message()
end
local flushed, stream_flush_err = stream:flush()
if stream_flush_err ~= nil then
    return stream_flush_err:message()
end
local stat, stat_err = stream:stat()
if stat_err ~= nil then
    return stat_err:message()
end
local closed, close_err = stream:close()
if close_err ~= nil then
    return close_err:message()
end

local form, form_err = request:parse_multipart(1048576)
if form_err ~= nil then
    return form_err:message()
end
local default_form, default_form_err = configured:parse_multipart()
if default_form_err ~= nil then
    return default_form_err:message()
end

-- A parsed form declares both of its collections as optional, so each is
-- tested before it is read.
local field_count: number = 0
if form.values ~= nil then
    for _, values in pairs(form.values) do
        field_count = field_count + #values
    end
end

local upload_bytes: number = 0
local upload_names: string = ""
if default_form.files ~= nil then
    for _, files in pairs(default_form.files) do
        for _, file in ipairs(files) do
            local size, size_err = file:size()
            if size_err ~= nil then
                return size_err:message()
            end
            upload_bytes = upload_bytes + size

            local name, name_err = file:name()
            if name_err ~= nil then
                return name_err:message()
            end
            upload_names = upload_names .. name

            local file_stream, file_stream_err = file:stream()
            if file_stream_err ~= nil then
                return file_stream_err:message()
            end
            local file_closed, file_close_err = file_stream:close()
            if file_close_err ~= nil then
                return file_close_err:message()
            end
            if not file_closed then
                return "upload stream stayed open"
            end

            -- MultipartFile.header answers an optional string and no error at
            -- all, so the nil is the whole result a caller must handle.
            local disposition = file:header("Content-Disposition")
            if disposition ~= nil then
                upload_names = upload_names .. disposition
            end
        end
    end
end

local status_err = response:set_status(http.STATUS.OK)
if status_err ~= nil then
    return status_err:message()
end

local header_err = response:set_header("X-Request-Id", id_text)
if header_err ~= nil then
    return header_err:message()
end

local content_type_err = response:set_content_type(http.CONTENT.JSON)
if content_type_err ~= nil then
    return content_type_err:message()
end

local transfer_err = response:set_transfer(http.TRANSFER.CHUNKED)
if transfer_err ~= nil then
    return transfer_err:message()
end

local body_write_err = response:write(body)
if body_write_err ~= nil then
    return body_write_err:message()
end

local json_write_err = response:write_json(document)
if json_write_err ~= nil then
    return json_write_err:message()
end

local event_err = response:write_event({ kind = "progress", bytes = upload_bytes })
if event_err ~= nil then
    return event_err:message()
end

local flush_err = response:flush()
if flush_err ~= nil then
    return flush_err:message()
end

if not (has_body and accepts_json and is_json and flushed and closed) then
    return "incomplete request"
end
return methods .. content .. transfer .. verb .. path .. id_text .. accept_text ..
    media_text .. slug_text .. host .. remote .. rest .. upload_names ..
    tostring(ok_status + redirect_status + client_status + server_status) ..
    tostring(length + written + offset + rewound + field_count) ..
    tostring(query_params) .. tostring(params) .. tostring(stat)
