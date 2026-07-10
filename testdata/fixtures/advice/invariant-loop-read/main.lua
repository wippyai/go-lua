type Config = { limit: number, step: number }

local config: Config = { limit = 3, step = 1 }
local total = 0
local i = 0
while i < 3 do
  local limit = config.limit
  total = total + limit
  i = i + 1
end

local changed: Config = { limit = 2, step = 1 }
local changed_alias = changed
while total < 10 do
  local limit = changed.limit
  if total > 5 then
    changed_alias.limit = limit + 1
  end
  total = total + 1
end

return total
