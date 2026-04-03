local constants = require("constants")

type FinishReasonMap = {[string]: string}
type ErrorTypeMap = {[string]: string}
type StatusCodeMap = {[number]: string}

local M = {}

M.finish_reasons: FinishReasonMap = {}
M.finish_reasons["end_turn"] = constants.FINISH_REASON.STOP
M.finish_reasons["max_tokens"] = constants.FINISH_REASON.LENGTH
M.finish_reasons["stop_sequence"] = constants.FINISH_REASON.STOP
M.finish_reasons["tool_use"] = constants.FINISH_REASON.TOOL_CALL

M.error_types: ErrorTypeMap = {}
M.error_types["invalid_request_error"] = constants.ERROR_TYPE.INVALID_REQUEST
M.error_types["authentication_error"] = constants.ERROR_TYPE.AUTHENTICATION
M.error_types["rate_limit_error"] = constants.ERROR_TYPE.RATE_LIMIT
M.error_types["api_error"] = constants.ERROR_TYPE.SERVER_ERROR

M.status_codes: StatusCodeMap = {}
M.status_codes[400] = constants.ERROR_TYPE.INVALID_REQUEST
M.status_codes[401] = constants.ERROR_TYPE.AUTHENTICATION
M.status_codes[429] = constants.ERROR_TYPE.RATE_LIMIT
M.status_codes[500] = constants.ERROR_TYPE.SERVER_ERROR

function M.map_finish_reason(api_reason: string): string
    return M.finish_reasons[api_reason] or "unknown"
end

function M.map_error_type(api_error: string): string
    return M.error_types[api_error] or constants.ERROR_TYPE.SERVER_ERROR
end

function M.map_status_code(code: number): string
    return M.status_codes[code] or constants.ERROR_TYPE.SERVER_ERROR
end

return M
