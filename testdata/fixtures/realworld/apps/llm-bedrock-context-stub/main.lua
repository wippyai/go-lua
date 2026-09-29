-- framework/src/llm/src/bedrock/client_test.lua: a context stub called with
-- different table shapes, including an empty table.
local bedrock_client = { _ctx = nil :: any }

local function use_context(context)
    bedrock_client._ctx = {
        all = function()
            return context
        end
    }
end

use_context({ retry = { attempts = 2, backoff_ms = 0 } })
use_context({})
use_context({ retry = { attempts = 1, backoff_ms = 0 } })
return bedrock_client
