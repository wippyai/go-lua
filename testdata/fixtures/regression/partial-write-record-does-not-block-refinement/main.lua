local mapper = require("google_mapper")
local google_function_calls = {
    {
        functionCall = {
            name = "get_weather",
            args = { location = "New York", units = "celsius" }
        }
    },
    {
        functionCall = {
            name = "calculate",
            args = { expression = "2+2" }
        }
    }
}
local contract_tool_calls = mapper.map_tool_calls(google_function_calls)
assert(#contract_tool_calls == 2)
assert(contract_tool_calls[1].name == "get_weather")
assert(contract_tool_calls[2].name == "calculate")
assert(contract_tool_calls[1].arguments.location == "New York")
