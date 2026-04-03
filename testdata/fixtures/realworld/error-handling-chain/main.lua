local errors = require("errors")
local validator = require("validator")

local result = validator.validate_name("Alice")
if result.ok then
    local name: string = result.value
else
    local err: AppError = result.error
    local wrapped = errors.wrap(err, "registration")
    local code: string = wrapped.code
    local msg: string = wrapped.message
    local retry: boolean = errors.is_retryable(wrapped)
end
