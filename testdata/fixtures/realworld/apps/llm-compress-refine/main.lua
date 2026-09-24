-- framework/src/llm/src/util/compress.lua: refine_length passes its own
-- parameters to its recursive call; the parameter types come from the outer
-- call site and are not erased by the recursion.
local CONFIG = { max_refinement_attempts = 3, min_length_tolerance = 10, length_tolerance_ratio = 0.1 }

local function refine_length(result, target_chars, model_name, options, attempts)
    attempts = tonumber(attempts) or 1
    local max_attempts = options.max_attempts or CONFIG.max_refinement_attempts

    if attempts > max_attempts then
        return result, nil
    end

    local actual_chars = #result
    local target: number = target_chars
    local tolerance = math.max(
        CONFIG.min_length_tolerance,
        math.floor(target_chars * CONFIG.length_tolerance_ratio)
    )

    if math.abs(actual_chars - target_chars) <= tolerance then
        return result, nil
    end

    return refine_length(result .. "x", target_chars, model_name, options, attempts + 1)
end

local function to_size(model_name: string, content: string, target_chars: number, options)
    options = options or {}
    return refine_length(content, target_chars, model_name, options)
end

return to_size
