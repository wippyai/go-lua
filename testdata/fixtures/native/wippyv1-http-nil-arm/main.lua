-- Contract: the Wippy v1 http surface declares seven results that carry a nil
-- the caller must handle - four Request readers and MultipartFile.header,
-- which answer an optional string, and the two collections of a parsed
-- multipart form, which are optional fields. Each one is read here twice: once
-- unguarded, where the nil reaches a consumer that cannot take it, and once
-- behind the test that rules the nil out. The guarded read is the whole point
-- of the declaration, so it must be silent.

local http = require("http")

local request, request_err = http.request()
if request_err ~= nil then
    return request_err:kind()
end

-- A query parameter the caller did not send has no value.
local raw_id: string = request:query("id") -- expect-error: may be nil
local id: string = "anonymous"
local queried, query_err = request:query("id")
if query_err == nil and queried ~= nil then
    id = queried
end

-- A header the client did not send has no value.
local raw_accept: string = request:header("Accept") -- expect-error: may be nil
local accept: string = "*/*"
local sent, header_err = request:header("Accept")
if header_err == nil and sent ~= nil then
    accept = sent
end

-- A request without a body declares no media type.
local raw_media: string = request:content_type() -- expect-error: may be nil
local media: string = http.CONTENT.TEXT
local declared, media_err = request:content_type()
if media_err == nil and declared ~= nil then
    media = declared
end

-- A route placeholder the request did not match has no value.
local raw_slug: string = request:param("slug") -- expect-error: may be nil
local slug: string = "index"
local matched, param_err = request:param("slug")
if param_err == nil and matched ~= nil then
    slug = matched
end

local form, form_err = request:parse_multipart()
if form_err ~= nil then
    return form_err:message()
end

-- A form that carried no plain fields has no values collection, and one that
-- carried no uploads has no files collection.
local raw_values = form.values
local raw_field_count: number = #raw_values -- expect-error: may be nil
local field_count: number = 0
if form.values ~= nil then
    for _, values in pairs(form.values) do
        field_count = field_count + #values
    end
end

local raw_files = form.files
local raw_upload_count: number = #raw_files -- expect-error: may be nil
local names: string = ""
if form.files ~= nil then
    for _, files in pairs(form.files) do
        for _, file in ipairs(files) do
            -- MultipartFile.header answers an optional string and no error at
            -- all, so the nil is the whole result.
            local raw_disposition: string = file:header("Content-Disposition") -- expect-error: may be nil
            local disposition = file:header("Content-Disposition")
            if disposition ~= nil then
                names = names .. disposition
            end
            names = names .. raw_disposition
        end
    end
end

return id .. accept .. media .. slug .. names .. raw_id .. raw_accept ..
    raw_media .. raw_slug ..
    tostring(field_count + raw_field_count + raw_upload_count)
