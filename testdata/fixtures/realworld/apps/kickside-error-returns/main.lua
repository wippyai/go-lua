-- kickside/core error-returning functions (jobs/persist/claim.lua,
-- threads/provision.lua, projections/persist/catchup.lua) with their
-- `:: error` casts removed: errors.new values fill `error?` returns directly.
type Job = { id: string, attempts: integer }

local function begin(): (string?, error?)
    return "tx", nil
end

local function select_next_pending(tx: string): (Job?, error?)
    if tx == "" then
        return nil, errors.new({ message = "no transaction", kind = errors.INVALID })
    end
    return { id = "job-1", attempts = 0 }, nil
end

local function claim(): (Job?, error?)
    local tx, tx_err = begin()
    if tx_err or not tx then
        return nil, errors.new({ message = "claim begin failed: " .. tostring(tx_err), kind = errors.INTERNAL })
    end
    local job, select_err = select_next_pending(tx)
    if select_err then
        return nil, select_err
    end
    if not job then
        return nil, nil
    end
    return job, nil
end

local function apply_grants(grants: { any }): error?
    if #grants == 0 then
        return nil
    end
    return errors.new({ message = "component service unavailable", kind = errors.INTERNAL, retryable = true })
end

local function encode_projection(body: { [string]: any }, current_last: integer): (integer, string, error?)
    local body_json, body_encode_err = json.encode(body)
    if body_encode_err or not body_json then
        return current_last, "", errors.new({
            message = "failed to encode projection body: " .. tostring(body_encode_err),
            kind = errors.INVALID,
            details = { last = current_last },
        })
    end
    return current_last + 1, body_json, nil
end

local job, claim_err = claim()
if claim_err then
    print(claim_err:kind(), claim_err:message())
elseif job then
    local attempts: integer = job.attempts
    print(attempts)
end

local grant_err = apply_grants({})
if grant_err and grant_err:retryable() then
    print("retry")
end

local last, encoded, encode_err = encode_projection({ a = 1 }, 3)
if encode_err then
    print(encode_err:message())
else
    local next_last: integer = last
    local text: string = encoded
    print(next_last, text)
end
