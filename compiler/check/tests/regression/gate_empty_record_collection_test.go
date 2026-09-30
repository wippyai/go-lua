package regression

import "testing"

func TestGateEmptyArrayAlternativePreservesAlias(t *testing.T) {
	checkBothModes(t, `
type Related = {outgoing: {number}}
local function attach(flag: boolean, r: Related)
 local e = {}
 local related: Related = flag and r or {outgoing = e}
 local function push(t: {string}) table.insert(t, "s") end
 push(e)
 local n: number = related.outgoing[1]
 return n
end
return attach`, "cannot assign")
}

func TestGateFreshArrayAlternativeUsesExpectedType(t *testing.T) {
	checkBothModes(t, `
type Related = {outgoing: {number}}
local function attach(flag: boolean, r: Related)
 local related: Related = flag and r or {outgoing = {}}
 local n: number = related.outgoing[1]
 return n
end
return attach`, "")
}

func TestGateBothArrayAlternativesUseExpectedType(t *testing.T) {
	checkBothModes(t, `
type Related = {outgoing: {number}}
local function attach(flag: boolean)
 local related: Related = flag and {outgoing = {}} or {outgoing = {1}}
 return related
end
return attach`, "")
}

func TestGateFirstArrayAlternativeRejectsWrongElement(t *testing.T) {
	checkBothModes(t, `
type Related = {outgoing: {number}}
local function attach(flag: boolean)
 local related: Related = flag and {outgoing = {"s"}} or {outgoing = {}}
 return related
end
return attach`, "cannot assign")
}

func TestGateDirectEmptyArrayFieldPreservesAlias(t *testing.T) {
	checkBothModes(t, `
type Related = {outgoing: {number}}
local e = {}
local related: Related = {outgoing = e}
local function push(t: {string}) table.insert(t, "s") end
push(e)
return related`, "cannot assign")
}

func TestGateRelatedEmptyArrayAlternative(t *testing.T) {
	checkBothModes(t, `
type Association = {attr: unknown, target: unknown}
type Related = {outgoing: {Association}?, incoming: {Association}?}
local function attach(raw: unknown)
 local related: Related = type(raw) == "table" and raw :: Related or {outgoing = {}, incoming = {}}
 return related
end
return attach`, "")
}

func TestGateRelatedRejectsWrongPresentElement(t *testing.T) {
	checkBothModes(t, `
type Association = {attr: string}
type Related = {outgoing: {Association}?}
local function attach(raw: unknown)
 local related: Related = type(raw) == "table" and raw :: Related or {outgoing = {false}}
 return related
end
return attach`, "cannot assign")
}
