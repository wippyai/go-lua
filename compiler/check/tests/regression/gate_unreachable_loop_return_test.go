package regression

import "testing"

func TestInfiniteLoopExitDoesNotReadPendingPhi(t *testing.T) {
	checkBothModes(t, `
local function run(flag: boolean): string
 local result = ""
 while true do
  if flag then return result end
  result = result .. "x"
 end
 return result
end
return run`, "")
}

func TestReachableLoopExitChecksReturnType(t *testing.T) {
	checkBothModes(t, `
local function run(flag: boolean): string
 local result = 1
 while true do
  if flag then break end
  result = result + 1
 end
 return result
end
return run`, "cannot return")
}
