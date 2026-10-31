-- kickside/spiralscout/market-watch/src/admission/fold.lua: validate_input
-- returns several values and a trailing error; once the error path returned,
-- every value is present.
type Map = { [string]: any }
type List = { any }

local function object(value: any, path: string): (Map?, string?)
    if type(value) ~= "table" then return nil, path .. " must be an object" end
    return value :: Map, nil
end

local function validate_input(value: any): (Map?, Map?, Map?, List?, string?)
    local input, input_err = object(value, "admission")
    if not input then return nil, nil, nil, nil, input_err end
    local observation, observation_err = object(input.observation, "admission.observation")
    if not observation then return nil, nil, nil, nil, observation_err end
    local signal, signal_err = object(input.signal, "admission.signal")
    if not signal then return nil, nil, nil, nil, signal_err end
    local evidence: List = {}
    return input, observation, signal, evidence, nil
end

local function identity_keys(input: Map, signal: Map, observation: Map): (Map?, string?)
    return { signal_id = tostring(signal.id), scope = tostring(input.workspace_ref), observed = observation.proposal_id }, nil
end

local function classify(keys: Map, signal: Map, observation: Map, evidence: List): string
    if #evidence > 0 and keys.signal_id ~= nil and signal.kind ~= nil and observation.kind ~= nil then
        return "novel"
    end
    return "repeat"
end

local function admit(value: any): (Map?, string?)
    local input, observation, signal, evidence, input_err = validate_input(value)
    if not input then return nil, input_err end
    local keys, keys_err = identity_keys(input, signal, observation)
    if not keys then return nil, keys_err end
    return { novelty = classify(keys, signal, observation, evidence) }, nil
end

return admit
