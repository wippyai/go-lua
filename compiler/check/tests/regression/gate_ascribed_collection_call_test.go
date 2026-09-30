package regression

import "testing"

func TestAscribedCollectionReadAfterCallbackWrite(t *testing.T) {
	checkBothModes(t, `
local function emit(node: any)
 node:command({payload = {title = "ready"}})
end
local function run()
 local node = {command = function(self: any, cmd: any)
  self.commands = self.commands or {}
  table.insert(self.commands, cmd)
 end}
 (node :: any).commands = {}
 emit(node)
 return (node :: any).commands[1].payload
end
return run`, "")
}

func TestAscribedCollectionReadBeforeCallbackStillChecksEmptyValue(t *testing.T) {
	checkBothModes(t, `
local function run()
 local node = {}
 (node :: any).commands = {}
 return (node :: any).commands[1].payload
end
return run`, "cannot index type nil")
}

func TestAscribedScalarWriteDoesNotSurviveMutatingCall(t *testing.T) {
	checkBothModes(t, `
type T = {f: string?}
local function clear(t: any) t.f = nil end
local function run(raw: unknown)
 (raw :: T).f = "ready"
 clear(raw)
 local value: string = (raw :: T).f
 return value
end
return run`, "cannot assign")
}

func TestAscribedDeclaredScalarDomainAfterCall(t *testing.T) {
	checkBothModes(t, `
local function touch(t: any) return t end
local function run(raw: {f: string})
 touch(raw)
 (raw :: {f: string}).f = "ready"
 local value: string = (raw :: {f: string}).f
 return value
end
return run`, "")
}
