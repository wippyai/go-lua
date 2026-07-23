local typed: string = "ready"
local redundant = typed :: string
local cast_call = string(typed)

local function dynamic_claim(unknown: any): string
  local still_needed = unknown :: string
  return still_needed
end

local function dynamic_call(unknown: any): string
  local casted = string(unknown)
  return casted
end

return redundant .. cast_call
