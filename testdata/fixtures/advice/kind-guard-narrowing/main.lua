local function narrowed(value: string | number): string
  if type(value) == "string" then
    if type(value) ~= "number" then
      return value
    end
  end
  return ""
end

local function written(value: string | number, other: number): string
  if type(value) == "string" then
    value = other
    if type(value) ~= "number" then
      return ""
    end
  end
  return ""
end

return narrowed("first") .. written("second", 2)
