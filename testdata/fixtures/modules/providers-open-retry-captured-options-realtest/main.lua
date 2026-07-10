local test = require("test")
local providers = require("providers")

type RetryOptions = {
    retry: {
        max_attempts: number,
        initial_delay: number,
    }?,
}

local captured_options: RetryOptions? = nil

local provider_with_retry = {
    id = "wippy.llm.provider:openai",
    kind = "registry.entry",
    meta = { type = "llm.provider", name = "openai", title = "OpenAI" },
    data = {
        driver = {
            id = "wippy.llm.binding:openai_driver",
            options = {
                api_key_env = "OPENAI_API_KEY",
                retry = { max_attempts = 5, initial_delay = 200 },
            },
        },
    },
}

providers._registry = {
    get = function(id)
        if id == "wippy.llm.provider:openai" then
            return provider_with_retry, nil
        end
        return nil, "not found"
    end,
    find = function(_query)
        return {provider_with_retry}, nil
    end,
}

providers._contract = {
    get = function(_contract_id)
        return {
            with_context = function(self, _context)
                return self
            end,
            with_options = function(self, opts: RetryOptions)
                captured_options = opts
                return self
            end,
            open = function(self, binding_id)
                return { _binding_id = binding_id }, nil
            end,
        }, nil
    end,
}

local instance, err = providers.open("wippy.llm.provider:openai")

test.is_nil(err, "open should succeed")
assert(instance)
test.not_nil(captured_options, "captured options expected")
local options = captured_options
assert(options)
test.not_nil(options.retry, "retry expected")
assert(options.retry)

local attempts: number = options.retry.max_attempts
local delay: number = options.retry.initial_delay

return attempts, delay
