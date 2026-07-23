local function redundant(value: string?): string
  if value ~= nil then
    if value ~= nil then
      return value
    end
  end
  return ""
end

local function needed(value: string?): string
  if value ~= nil then
    value = nil
    if value ~= nil then
      return value
    end
  end
  return ""
end

local function typed(value: string | number): string
  if type(value) == "string" then
    if type(value) ~= "number" then
      return value
    end
  end
  return ""
end

return 0
