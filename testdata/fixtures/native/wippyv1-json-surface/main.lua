-- Contract: the Wippy v1 json module as the runtime declares it. All four
-- members are called and each is read on both arms of its trailing error.

local json = require("json")

local encoded, encode_err = json.encode({ id = 7, name = "widget" })
if encode_err ~= nil then
    return encode_err:kind()
end

local decoded, decode_err = json.decode(encoded)
if decode_err ~= nil then
    return decode_err:message()
end

-- The schema parameter admits a table structure or a string reference, and the
-- module declares both forms under one union.
local schema = { type = "object" }
local valid, validate_err = json.validate(schema, decoded)
if validate_err ~= nil then
    return validate_err:message()
end

local named_valid, named_err = json.validate("widget.schema", decoded)
if named_err ~= nil then
    return named_err:message()
end

local text_valid, text_err = json.validate_string(schema, encoded)
if text_err ~= nil then
    return text_err:message()
end

local named_text_valid, named_text_err = json.validate_string("widget.schema", encoded)
if named_text_err ~= nil then
    return named_text_err:message()
end

if not (valid and named_valid and text_valid and named_text_valid) then
    return "invalid"
end
return encoded
