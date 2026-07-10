local function build_delta(tool: boolean)
  local delta = {}
  if tool then
    delta.tool_call = "run"
    delta.args = { source = "chat" }
  else
    delta.content = "hello"
  end
  return delta
end

local uniform = { kind = "content", content = "hello" }
local stable_m = { name = "module", version = 1 }

local dictionary = {}
local dynamic_key = "entry"
dictionary[dynamic_key] = 1
for key, value in pairs(dictionary) do
  dictionary[key] = value
end

local fixed_event = {
  type = "delta",
  tool_call = nil,
  args = nil,
  content = nil,
  id = nil,
  role = nil,
  finish_reason = nil,
  metadata = nil,
}

local delta = build_delta(true)
return delta.content or uniform.content or stable_m.name or fixed_event.type
