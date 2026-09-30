package regression

import "testing"

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
