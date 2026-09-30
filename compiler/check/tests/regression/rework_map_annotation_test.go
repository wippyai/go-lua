package regression

import "testing"

func TestRefinedMapAnnotationRetainsDomainAfterFieldWrites(t *testing.T) {
	checkBothModes(t, `
local function empty_object(): {[string]: any}
 local obj: {[string]: any} = {}
 obj.placeholder = true
 obj.placeholder = nil
 return obj
end
local function copy(value: {[string]: unknown}): {[string]: unknown}
 local out: {[string]: unknown} = {}
 for key, item in pairs(value) do out[key] = item end
 out.host = {security = {"default"}}
 return out
end
return empty_object, copy`, "")
}

func TestRefinedMapAnnotationRejectsWrongKey(t *testing.T) {
	checkBothModes(t, `
local out: {[string]: any} = {}
out[1] = true
return out`, "cannot assign")
}

func TestRefinedMapAnnotationDoesNotNarrowUnknownValue(t *testing.T) {
	checkBothModes(t, `
local function run(source: {[string]: unknown}): string
 local out: {[string]: unknown} = source
 out.present = "s"
 return out.other
end
return run`, "cannot return")
}
