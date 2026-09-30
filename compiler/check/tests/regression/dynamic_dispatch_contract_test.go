package regression

import "testing"

func TestValueSourceExplicitAnyDispatch(t *testing.T) {
	checkBothModes(t, `
local function decode(row: any): any return row end
local entities = {one = {decode = decode}}
local function read(entity: string, row: any)
    local spec: any = entities[entity]
    return spec.decode(row)
end
return read`, "cannot call optional")
}

func TestValueSourceDispatchPresenceControls(t *testing.T) {
	checkBothModes(t, `
local entities = {one = {decode = function(row: any): any return row end}}
local function read(entity: string, row: any)
 local spec = entities[entity]
 if not spec then return nil end
 return spec.decode(row)
end
return read`, "")
	checkBothModes(t, `
local entities = {one = {decode = function(row: any): any return row end}}
local function read(entity: "one", row: any)
 return entities[entity].decode(row)
end
return read`, "")
}

func TestValueSourceUncorrelatedMapPresence(t *testing.T) {
	checkBothModes(t, `
type Info = {handler: () -> number}
local function run(ids: {[string]: string}, channels: {[string]: Info}, channel: string)
 local id = ids[channel]
 if id then
  local info = channels[id]
  return info.handler()
 end
 return nil
end
return run`, "cannot call optional")
	checkBothModes(t, `
type Info = {handler: () -> number}
local function run(ids: {[string]: string}, channels: {[string]: Info}, channel: string)
 local id = ids[channel]
 if id then
  local info = channels[id]
  if info then return info.handler() end
 end
 return nil
end
return run`, "")
}
