local test = require("test")
local providers = require("providers")

type RetryOptions = {
    retry: {
        max_attempts: number,
        initial_delay: number,
    }?,
}

local captured_options: RetryOptions? = nil

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

local instance, err = providers.open("wippy.llm.provider:openai", {
    retry = { max_attempts = 3, initial_delay = 100 },
})

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
