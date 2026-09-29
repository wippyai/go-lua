-- From kickside.automation:failure in kickside/platform/transform/test.
local errors = {
    INVALID = "invalid", NOT_FOUND = "not_found", CONFLICT = "conflict",
    PERMISSION_DENIED = "permission_denied", UNAVAILABLE = "unavailable",
}

local KIND_CODES: { [string]: string } = {
    [tostring(errors.INVALID)] = "invalid_request",
    [tostring(errors.NOT_FOUND)] = "not_found",
    [tostring(errors.CONFLICT)] = "conflict",
    [tostring(errors.PERMISSION_DENIED)] = "permission_denied",
    [tostring(errors.UNAVAILABLE)] = "provider_unavailable",
}

local BAD_KEYS: { [string]: string } = { -- expect-error: field key type mismatch
    [1] = "wrong",
}

local BAD_VALUES: { [string]: string } = { -- expect-error: field value type mismatch
    ["wrong"] = 1,
}

return { kind_codes = KIND_CODES, bad_keys = BAD_KEYS, bad_values = BAD_VALUES }
