package regression

import "testing"

func TestClassCollectionEstimateRetainsAssignedElementDomain(t *testing.T) {
	checkBothModes(t, `
type Item = {id: string}
local C = {}
C.__index = C
function C.new(): any
 local self = setmetatable({}, C)
 self.values = {}
 return self
end
function C:set(values: {Item}) self.values = values end
function C:request()
 local request: {values: {Item}} = {values = self.values or {}}
 return request
end
return C`, "")
}

func TestClassCollectionEstimateRejectsWrongElementDomain(t *testing.T) {
	checkBothModes(t, `
type Item = {id: string}
local C = {}
C.__index = C
function C.new(): any
 local self = setmetatable({}, C)
 self.values = {}
 return self
end
function C:set(values: {Item}) self.values = values end
function C:request()
 local request: {values: {number}} = {values = self.values or {}}
 return request
end
return C`, "cannot assign")
}

func TestClassCollectionEstimateRejectsSharedEmptyInitializer(t *testing.T) {
	checkBothModes(t, `
type Item = {id: string}
local C = {}
C.__index = C
local shared = {}
function C.new(): any
 local self = setmetatable({}, C)
 self.values = shared
 return self
end
function C:set(values: {Item}) self.values = values end
local function push(t: {string}) table.insert(t, "wrong") end
push(shared)
function C:request()
 local request: {values: {Item}} = {values = self.values or {}}
 return request
end
return C`, "cannot assign")
}

func TestClassCollectionEstimateRejectsConflictingMethodWrites(t *testing.T) {
	checkBothModes(t, `
type Item = {id: string}
local C = {}
C.__index = C
function C.new(): any
 local self = setmetatable({}, C)
 self.values = {}
 return self
end
function C:set(values: {Item}) self.values = values end
function C:replace(values: {number}) self.values = values end
function C:request()
 local request: {values: {Item}} = {values = self.values or {}}
 return request
end
return C`, "cannot assign")
}

func TestClassCollectionEstimateRejectsSharedMethodInitializer(t *testing.T) {
	checkBothModes(t, `
type Item = {id: string}
local C = {}
C.__index = C
local shared = {}
function C.new(): any
 local self = setmetatable({}, C)
 self.values = {}
 return self
end
function C:set(values: {Item}) self.values = values end
function C:replace() self.values = shared end
local function push(t: {string}) table.insert(t, "wrong") end
push(shared)
function C:request()
 local request: {values: {Item}} = {values = self.values or {}}
 return request
end
return C`, "cannot assign")
}
