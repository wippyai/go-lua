local function sealedReader(value: string | number): string
  if type(value) == "string" then
    if type(value) ~= "number" then
      return value
    end
  end
  return ""
end

local function mutatedReader(): string
  if type(subject) == "string" then
    return subject
  end
  return ""
end

subject = "written by this program"
return 0
