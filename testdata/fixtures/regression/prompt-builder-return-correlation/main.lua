-- From wippy.session:prompt_builder_test, first provider_metadata case.
local test = require("test")
local json = require("json")
local consts = require("consts")
local prompt_builder = require("prompt_builder")

local messages = {
    {
        message_id = "msg-1",
        type = consts.MSG_TYPE.FUNCTION,
        data = json.encode({ query = "test" }),
        metadata = {
            function_name = "search",
            call_id = "call-1",
            registry_id = "reg-1",
            status = consts.FUNC_STATUS.SUCCESS,
            result = "found it",
            provider_metadata = {
                anthropic = { citations = { enabled = true } }
            }
        }
    }
}

local builder, err = prompt_builder.build(messages, {}, {}, {
    include_contexts = false,
    include_files = false,
    cache_markers = false
})

test.is_nil(err)
local built = builder:get_messages()
test.ok(#built >= 2)
