local function called(value: string | number): string
  if type(value) == "string" then
    if type(value) ~= "number" then
      return value
    end
  end
  return ""
end

local function uncalled(value: string | number): string
  if type(value) == "string" then
    if type(value) ~= "number" then
      return value
    end
  end
  return ""
end

return called("first")
