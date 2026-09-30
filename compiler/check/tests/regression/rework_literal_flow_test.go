package regression

import "testing"

func TestInferredLiteralRetainsCompleteKeyEvidence(t *testing.T) {
	checkBothModes(t, `
type Map = {[string]: any}
local function f(): Map
 local value = {id = "s"}
 value.title = "new"
 return value
end
local function g(): Map
 local value = {}
 value.title = "new"
 return value
end
return f, g`, "")
}

func TestPartialInputRetainsUnknownKeyEvidence(t *testing.T) {
	checkBothModes(t, `
local function f(value: {id: string}): {[string]: any}
 value.id = "new"
 return value
end
return f`, "cannot return")
}
