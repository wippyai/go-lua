local bedrock_client = require("bedrock_client")

-- The body forwards model_id into client.invoke, whose first parameter is typed
-- string. That use proves model_id: string for the unannotated helper, so passing
-- an `any` (a dynamic field read) through the helper is rejected at the call site.
local function helper(client, model_id)
    return client.invoke(model_id, {}, {})
end

local contract_args = nil :: any
local model_id = contract_args.model
helper(bedrock_client, model_id) -- expect-error: not string
