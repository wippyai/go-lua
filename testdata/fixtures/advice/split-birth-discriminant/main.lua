local function compute(): number
  return 41
end

local split = {}
split.kind = "ok"
split.value = compute()

local split_score = 0
if split.kind == "ok" then
  split_score = split.value
end

local atomic = { kind = "ok", value = compute() }
local atomic_score = 0
if atomic.kind == "ok" then
  atomic_score = atomic.value
end

local ordinary = {}
ordinary.kind = "ok"
ordinary.value = compute()
local ordinary_score = ordinary.value

return split_score + atomic_score + ordinary_score
