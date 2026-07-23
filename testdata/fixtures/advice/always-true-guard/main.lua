local flag = true
if flag then
  flag = true
else
  flag = false
end

local function check(maybe: boolean): boolean
  if maybe then
    return true
  end
  return false
end

return check(flag)
